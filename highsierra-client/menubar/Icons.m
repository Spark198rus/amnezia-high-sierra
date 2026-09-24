#import "Icons.h"

static const CGFloat AWGIconSize = 18;

static NSBezierPath *AWGShieldPath(void) {
    NSBezierPath *p = [NSBezierPath bezierPath];
    [p moveToPoint:NSMakePoint(9, 16.5)];
    [p curveToPoint:NSMakePoint(3.5, 14.2) controlPoint1:NSMakePoint(7.2, 15.2) controlPoint2:NSMakePoint(5.3, 14.4)];
    [p lineToPoint:NSMakePoint(3.5, 9)];
    [p curveToPoint:NSMakePoint(9, 1.5) controlPoint1:NSMakePoint(3.5, 5.2) controlPoint2:NSMakePoint(6, 2.8)];
    [p curveToPoint:NSMakePoint(14.5, 9) controlPoint1:NSMakePoint(12, 2.8) controlPoint2:NSMakePoint(14.5, 5.2)];
    [p lineToPoint:NSMakePoint(14.5, 14.2)];
    [p curveToPoint:NSMakePoint(9, 16.5) controlPoint1:NSMakePoint(12.7, 14.4) controlPoint2:NSMakePoint(10.8, 15.2)];
    [p closePath];
    p.lineWidth = 1.4;
    return p;
}

static void AWGDrawIcon(AWGIconState state) {
    NSBezierPath *shield = AWGShieldPath();
    [[NSColor blackColor] set];

    switch (state) {
    case AWGIconConnected:
        [shield fill];
        break;

    case AWGIconBusy:
        // Half full: something is happening.
        [shield stroke];
        [NSGraphicsContext saveGraphicsState];
        [NSBezierPath clipRect:NSMakeRect(0, 0, AWGIconSize, 9)];
        [shield fill];
        [NSGraphicsContext restoreGraphicsState];
        break;

    case AWGIconBlocked: {
        // An exclamation mark: the kill switch is holding traffic back.
        [shield stroke];
        NSBezierPath *bar = [NSBezierPath bezierPathWithRoundedRect:NSMakeRect(8.2, 7.2, 1.6, 5.8) xRadius:0.8 yRadius:0.8];
        [bar fill];
        [[NSBezierPath bezierPathWithOvalInRect:NSMakeRect(8.1, 4.3, 1.8, 1.8)] fill];
        break;
    }

    case AWGIconUnavailable: {
        [shield stroke];
        NSBezierPath *slash = [NSBezierPath bezierPath];
        [slash moveToPoint:NSMakePoint(2, 2)];
        [slash lineToPoint:NSMakePoint(16, 16)];
        slash.lineWidth = 1.4;
        [slash stroke];
        break;
    }

    case AWGIconDisconnected:
        [shield stroke];
        break;
    }
}

NSImage *AWGStatusIcon(AWGIconState state) {
    static NSMutableDictionary<NSNumber *, NSImage *> *cache;
    if (!cache) {
        cache = [NSMutableDictionary dictionary];
    }
    NSImage *image = cache[@(state)];
    if (!image) {
        image = [NSImage imageWithSize:NSMakeSize(AWGIconSize, AWGIconSize)
                               flipped:NO
                        drawingHandler:^BOOL(NSRect rect) {
                            AWGDrawIcon(state);
                            return YES;
                        }];
        [image setTemplate:YES];
        cache[@(state)] = image;
    }
    return image;
}
