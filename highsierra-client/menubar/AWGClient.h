#import <Foundation/Foundation.h>

NS_ASSUME_NONNULL_BEGIN

// AWGCall sends one request to the awg-hs service over its Unix socket and
// returns the reply, or nil with *error set. It blocks, so call it off the
// main thread. The protocol is internal/control/control.go.
NSDictionary *_Nullable AWGCall(NSDictionary *request, NSError **error);

// AWGDescribeError turns an AWGCall error into a sentence for the user.
NSString *AWGDescribeError(NSError *_Nullable error);

NS_ASSUME_NONNULL_END
