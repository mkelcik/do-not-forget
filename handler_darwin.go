package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>
#include <stdbool.h>
#include <stdlib.h>

// Whether some installed app is registered to open this URL (Launch Services).
static bool has_url_handler(const char *link) {
	@autoreleasepool {
		NSURL *u = [NSURL URLWithString:[NSString stringWithUTF8String:link]];
		return u != nil && [[NSWorkspace sharedWorkspace] URLForApplicationToOpenURL:u] != nil;
	}
}
*/
import "C"

import "unsafe"

// hasURLHandler reports whether an installed app opens this native meeting link.
func hasURLHandler(native string) bool {
	c := C.CString(native)
	defer C.free(unsafe.Pointer(c))
	return bool(C.has_url_handler(c))
}
