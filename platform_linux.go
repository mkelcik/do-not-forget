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
	"reflect"
	"unsafe"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// skipTaskbar must run after the window is shown: the WM drops _NET_WM_STATE on unmap.
func skipTaskbar(w application.Window) {
	application.InvokeSync(func() { C.skip_taskbar(unsafe.Pointer(w.NativeWindow())) })
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
	if err := p.Set("org.kde.StatusNotifierItem", "ToolTip", dbus.MakeVariant(tt)); err != nil {
		return
	}
	if conn, err := dbus.SessionBus(); err == nil {
		_ = conn.Emit("/StatusNotifierItem", "org.kde.StatusNotifierItem.NewToolTip")
	}
}
