package main

import "golang.org/x/sys/windows/registry"

// hasURLHandler reports whether an installed app opens this native meeting link: its scheme is
// registered as a URL protocol. Without one, Windows would ask the user to pick an app.
func hasURLHandler(native string) bool {
	k, err := registry.OpenKey(registry.CLASSES_ROOT, schemeOf(native), registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer func() { _ = k.Close() }()
	_, _, err = k.GetStringValue("URL Protocol")
	return err == nil
}
