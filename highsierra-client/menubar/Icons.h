#import <Cocoa/Cocoa.h>

typedef NS_ENUM(NSInteger, AWGIconState) {
    AWGIconDisconnected,
    AWGIconConnected,
    AWGIconBusy,        // connecting or disconnecting
    AWGIconBlocked,     // the kill switch is blocking without a tunnel
    AWGIconUnavailable, // the service can't be reached
};

// AWGStatusIcon returns the menu bar icon for state: a shield, drawn as a
// template image so macOS colors it for light and dark menu bars.
NSImage *AWGStatusIcon(AWGIconState state);
