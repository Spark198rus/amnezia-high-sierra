#import <Cocoa/Cocoa.h>

// AppDelegate runs the menu bar item. The tunnel itself belongs to the
// awg-hs service; this app only shows its state and sends it commands, so
// quitting the app leaves the VPN as it is.
@interface AppDelegate : NSObject <NSApplicationDelegate, NSMenuDelegate>
@end
