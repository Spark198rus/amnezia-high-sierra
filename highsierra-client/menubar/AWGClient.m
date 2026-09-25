#import "AWGClient.h"

#include <errno.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/time.h>
#include <sys/un.h>
#include <unistd.h>

static NSString *const AWGSocketPath = @"/var/run/awg-hs.sock";
static const NSUInteger AWGMaxReply = 1 << 20;

static NSError *AWGPOSIXError(int code) {
    return [NSError errorWithDomain:NSPOSIXErrorDomain code:code userInfo:nil];
}

static NSDictionary *AWGFail(int fd, int code, NSError **error) {
    close(fd);
    if (error) {
        *error = AWGPOSIXError(code);
    }
    return nil;
}

NSDictionary *AWGCall(NSDictionary *request, NSError **error) {
    NSData *json = [NSJSONSerialization dataWithJSONObject:request options:0 error:error];
    if (!json) {
        return nil;
    }
    NSMutableData *message = [json mutableCopy];
    [message appendBytes:"\n" length:1];

    int fd = socket(AF_UNIX, SOCK_STREAM, 0);
    if (fd < 0) {
        if (error) {
            *error = AWGPOSIXError(errno);
        }
        return nil;
    }
    int one = 1;
    setsockopt(fd, SOL_SOCKET, SO_NOSIGPIPE, &one, sizeof one);
    // Connecting can take a while: the service may look up the server's name.
    struct timeval timeout = {.tv_sec = 120, .tv_usec = 0};
    setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &timeout, sizeof timeout);
    setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &timeout, sizeof timeout);

    struct sockaddr_un addr;
    memset(&addr, 0, sizeof addr);
    addr.sun_family = AF_UNIX;
    strlcpy(addr.sun_path, AWGSocketPath.fileSystemRepresentation, sizeof addr.sun_path);
    if (connect(fd, (struct sockaddr *)&addr, sizeof addr) != 0) {
        return AWGFail(fd, errno, error);
    }

    const uint8_t *bytes = message.bytes;
    size_t left = message.length;
    while (left > 0) {
        ssize_t n = write(fd, bytes, left);
        if (n < 0 && errno == EINTR) {
            continue;
        }
        if (n <= 0) {
            return AWGFail(fd, n < 0 ? errno : EPIPE, error);
        }
        bytes += n;
        left -= (size_t)n;
    }

    // The reply is one line of JSON.
    NSMutableData *reply = [NSMutableData data];
    uint8_t buf[4096];
    for (;;) {
        ssize_t n = read(fd, buf, sizeof buf);
        if (n < 0 && errno == EINTR) {
            continue;
        }
        if (n < 0) {
            return AWGFail(fd, errno, error);
        }
        if (n == 0) {
            break;
        }
        [reply appendBytes:buf length:(NSUInteger)n];
        if (memchr(buf, '\n', (size_t)n) || reply.length > AWGMaxReply) {
            break;
        }
    }
    close(fd);

    id object = nil;
    if (reply.length > 0) {
        object = [NSJSONSerialization JSONObjectWithData:reply options:0 error:NULL];
    }
    if (![object isKindOfClass:[NSDictionary class]]) {
        if (error) {
            *error = [NSError errorWithDomain:@"AWGClient"
                                         code:1
                                     userInfo:@{NSLocalizedDescriptionKey : NSLocalizedString(@"The awg-hs service sent an unexpected reply.", nil)}];
        }
        return nil;
    }
    return object;
}

NSString *AWGDescribeError(NSError *error) {
    if ([error.domain isEqualToString:NSPOSIXErrorDomain]) {
        switch (error.code) {
        case ENOENT:
        case ECONNREFUSED:
            return NSLocalizedString(@"The awg-hs service isn't running. Installing awg-hs again (sudo ./install.sh) starts it.", nil);
        case EACCES:
        case EPERM:
            return NSLocalizedString(@"Only administrator accounts can control the VPN.", nil);
        case EAGAIN:
            return NSLocalizedString(@"The awg-hs service didn't answer in time.", nil);
        }
    }
    return error.localizedDescription ?: NSLocalizedString(@"Something went wrong talking to the awg-hs service.", nil);
}
