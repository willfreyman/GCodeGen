//go:build darwin

package ui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#import <stdlib.h>
#import <string.h>

// Captured file path from a kAEOpenDocuments Apple Event delivered at
// app launch (e.g. user double-clicks foo.nc in Finder, or right-clicks
// → Open With → GcodeSimV3). The Go side reads this once and uses it
// as the initial file to load.
static char *captured_path = NULL;

@interface GsOpenDocHandler : NSObject
@end

@implementation GsOpenDocHandler

- (void)handleOpenDocs:(NSAppleEventDescriptor *)event
        withReplyEvent:(NSAppleEventDescriptor *)reply {
    NSAppleEventDescriptor *items = [event paramDescriptorForKeyword:keyDirectObject];
    if (!items || items.numberOfItems < 1) return;

    NSAppleEventDescriptor *item = [items descriptorAtIndex:1];
    NSAppleEventDescriptor *urlDesc = [item coerceToDescriptorType:typeFileURL];
    if (!urlDesc) return;

    NSData *urlData = urlDesc.data;
    if (!urlData) return;

    NSString *urlStr = [[NSString alloc] initWithData:urlData encoding:NSUTF8StringEncoding];
    NSURL *url = [NSURL URLWithString:urlStr];
    if (!url) return;

    const char *path = [url.path UTF8String];
    if (!path) return;

    if (captured_path) free(captured_path);
    captured_path = strdup(path);
}

@end

static GsOpenDocHandler *handler = NULL;

// drain_open_event installs a kAEOpenDocuments handler and runs the
// NSApp event loop briefly so any pending Apple Events delivered by
// Launch Services on launch get processed. Safe to call before GLFW
// initializes — we just bring up NSApp (which GLFW will reuse) and
// register an event handler that doesn't conflict with NSApp's delegate.
static const char *drain_open_event() {
    @autoreleasepool {
        // Force NSApp into existence so the event manager has somewhere
        // to deliver. GLFW will reuse this same NSApp later.
        [NSApplication sharedApplication];

        if (!handler) {
            handler = [[GsOpenDocHandler alloc] init];
            [[NSAppleEventManager sharedAppleEventManager]
                setEventHandler:handler
                andSelector:@selector(handleOpenDocs:withReplyEvent:)
                forEventClass:kCoreEventClass
                andEventID:kAEOpenDocuments];
        }

        // Pump events for up to 200 ms or until we capture a path.
        NSDate *until = [NSDate dateWithTimeIntervalSinceNow:0.2];
        while (!captured_path && [until timeIntervalSinceNow] > 0) {
            NSEvent *e = [NSApp nextEventMatchingMask:NSEventMaskAny
                untilDate:[NSDate dateWithTimeIntervalSinceNow:0.02]
                inMode:NSDefaultRunLoopMode
                dequeue:YES];
            if (e) [NSApp sendEvent:e];
        }
    }
    return captured_path;
}
*/
import "C"

// CapturedOpenFile returns the file path Launch Services delivered to us
// on startup (double-clicked .nc, dragged onto the app icon, Open With...).
// Returns "" if we were launched normally. Call once before window.Init.
func CapturedOpenFile() string {
	p := C.drain_open_event()
	if p == nil {
		return ""
	}
	return C.GoString(p)
}
