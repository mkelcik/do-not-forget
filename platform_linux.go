package main

/*
#cgo pkg-config: gtk4 x11
#cgo CFLAGS: -Wno-deprecated-declarations
#include <gtk/gtk.h>
#include <gdk/x11/gdkx.h>

// Ask the WM to keep the window out of the taskbar and pager (EWMH _NET_WM_STATE).
static void skip_taskbar(void *win) {
	GdkSurface *surface = gtk_native_get_surface(GTK_NATIVE(win));
	if (!surface || !GDK_IS_X11_SURFACE(surface)) return;
	Display *d = gdk_x11_display_get_xdisplay(gdk_surface_get_display(surface));
	XEvent e = {0};
	e.xclient.type = ClientMessage;
	e.xclient.window = gdk_x11_surface_get_xid(surface);
	e.xclient.message_type = XInternAtom(d, "_NET_WM_STATE", False);
	e.xclient.format = 32;
	e.xclient.data.l[0] = 1; // _NET_WM_STATE_ADD
	e.xclient.data.l[1] = XInternAtom(d, "_NET_WM_STATE_SKIP_TASKBAR", False);
	e.xclient.data.l[2] = XInternAtom(d, "_NET_WM_STATE_SKIP_PAGER", False);
	e.xclient.data.l[3] = 1; // source: normal application
	XSendEvent(d, DefaultRootWindow(d), False, SubstructureRedirectMask | SubstructureNotifyMask, &e);
	XFlush(d);
}
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// skipTaskbar must run after the window is shown: the WM drops _NET_WM_STATE on unmap.
func skipTaskbar(w application.Window) {
	application.InvokeSync(func() { C.skip_taskbar(w.NativeWindow()) })
}

// setTrayTooltip sets the SNI ToolTip (Plasma renders title + rich-text subtitle).
// Wails' Linux setTooltip is a TBD no-op, so we reach its D-Bus props via reflection.
// ponytail: pinned to Wails v3.0.0-beta.30 internals, drop once upstream implements setTooltip.
func setTrayTooltip(tray *application.SystemTray, title, sub string) {
	impl := reflect.ValueOf(tray).Elem().FieldByName("impl")
	if !impl.IsValid() || impl.IsNil() {
		return // tray not registered yet
	}
	props := impl.Elem().Elem().FieldByName("props")
	if !props.IsValid() || props.IsNil() {
		return
	}
	p := (*prop.Properties)(unsafe.Pointer(props.Pointer()))
	// Same unnamed struct type as Wails' `tooltip` alias: (icon name, icons, title, subtitle).
	tt := struct {
		V0 string
		V1 []application.PX
		V2 string
		V3 string
	}{V2: title, V3: sub}
	if p.Set("org.kde.StatusNotifierItem", "ToolTip", dbus.MakeVariant(tt)) != nil {
		return
	}
	if conn, err := dbus.SessionBus(); err == nil {
		_ = conn.Emit("/StatusNotifierItem", "org.kde.StatusNotifierItem.NewToolTip")
	}
}

// flatpakAutostart asks the Background portal to add or remove the start-at-login entry: the
// sandbox can't write ~/.config/autostart, and Wails' Autostart would write into the sandbox.
func flatpakAutostart(on bool) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return err
	}
	// The reply arrives as a Response signal on a request path derived from our bus name and
	// handle_token; subscribe before calling so it can't be missed.
	token := fmt.Sprintf("autostart%d", time.Now().UnixNano())
	sender := strings.ReplaceAll(strings.TrimPrefix(conn.Names()[0], ":"), ".", "_")
	path := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/" + sender + "/" + token)
	match := []dbus.MatchOption{dbus.WithMatchObjectPath(path), dbus.WithMatchInterface("org.freedesktop.portal.Request"), dbus.WithMatchMember("Response")}
	err = conn.AddMatchSignal(match...)
	if err != nil {
		return err
	}
	defer func() { _ = conn.RemoveMatchSignal(match...) }()
	signals := make(chan *dbus.Signal, 8)
	conn.Signal(signals)
	defer conn.RemoveSignal(signals)

	opts := map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(token),
		"autostart":    dbus.MakeVariant(on),
		"commandline":  dbus.MakeVariant([]string{"do-not-forget"}),
	}
	desktop := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")
	err = desktop.Call("org.freedesktop.portal.Background.RequestBackground", 0, "", opts).Err
	if err != nil {
		return err
	}
	timeout := time.After(2 * time.Minute) // the portal may ask the user first
	for {
		select {
		case s := <-signals:
			if s.Path != path || len(s.Body) < 2 {
				continue
			}
			if code, _ := s.Body[0].(uint32); code != 0 {
				return fmt.Errorf("background portal refused (%d)", code)
			}
			results, _ := s.Body[1].(map[string]dbus.Variant)
			if granted, _ := results["autostart"].Value().(bool); granted != on {
				return errors.New("background portal did not change start at login")
			}
			return nil
		case <-timeout:
			// Close the dialog, or a late approval would change autostart after we reported failure.
			_ = conn.Object("org.freedesktop.portal.Desktop", path).Call("org.freedesktop.portal.Request.Close", 0).Err
			return errors.New("background portal did not answer")
		}
	}
}

// installedBy names the package format that owns our binary, "deb" or "rpm", so an update can
// go through its package manager. "" for anything else, e.g. the AUR's package (pacman's job).
func installedBy() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if exec.CommandContext(ctx, "dpkg", "-S", exe).Run() == nil { //nolint:gosec // G204: our own path
		return "deb"
	}
	if exec.CommandContext(ctx, "rpm", "-qf", exe).Run() == nil { //nolint:gosec // G204: our own path
		return "rpm"
	}
	return ""
}

// installPackage installs a verified package as root; pkexec asks for the password.
func installPackage(ctx context.Context, kind, path string) error {
	args := []string{"apt-get", "install", "-y", path}
	if kind == "rpm" {
		args = []string{"dnf", "install", "-y", path}
		if dnf, _ := exec.LookPath("dnf"); dnf == "" {
			// openSUSE; our RPMs carry no GPG signature, the manifest signature vouches for them.
			args = []string{"zypper", "--non-interactive", "install", "--allow-unsigned-rpm", path}
		}
	}
	out, err := exec.CommandContext(ctx, "pkexec", args...).CombinedOutput() //nolint:gosec // G204: fixed commands, path from downloadPackage
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 126 { // the password dialog was dismissed
		return errCancelled
	}
	if err != nil {
		return fmt.Errorf("pkexec %s: %w: %s", args[0], err, out)
	}
	return nil
}

// relaunch starts the updated binary once this process has exited, so two copies never run.
func relaunch() error {
	exe, err := os.Executable() // Go drops the " (deleted)" the replaced binary now has
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(context.Background(), "sh", "-c", `while kill -0 "$0" 2>/dev/null; do sleep 0.2; done; exec "$1"`,
		strconv.Itoa(os.Getpid()), exe) //nolint:gosec // G204: our own pid and path
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
