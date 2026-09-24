#import "AppDelegate.h"

#import "AWGClient.h"
#import "Icons.h"

static NSString *const AWGLoginAgentLabel = @"io.github.spark198rus.awg-hs.menu";
static NSString *const AWGLogPath = @"/Library/Logs/awg-hs.log";
static NSString *const AWGDidSetUpLoginKey = @"DidSetUpOpenAtLogin";
static const NSTimeInterval AWGPollInterval = 3;

// Readers for the service's JSON that tolerate missing keys and odd types.

static BOOL AWGBool(NSDictionary *d, NSString *key) {
    id v = d[key];
    return [v isKindOfClass:[NSNumber class]] && [v boolValue];
}

static NSString *AWGString(NSDictionary *d, NSString *key) {
    id v = d[key];
    return [v isKindOfClass:[NSString class]] ? v : @"";
}

static long long AWGNumber(NSDictionary *d, NSString *key) {
    id v = d[key];
    return [v isKindOfClass:[NSNumber class]] ? [v longLongValue] : 0;
}

static NSArray<NSString *> *AWGStrings(NSDictionary *d, NSString *key) {
    id v = d[key];
    NSMutableArray<NSString *> *out = [NSMutableArray array];
    if ([v isKindOfClass:[NSArray class]]) {
        for (id s in v) {
            if ([s isKindOfClass:[NSString class]]) {
                [out addObject:s];
            }
        }
    }
    return out;
}

static NSString *AWGBytes(long long n) {
    return [NSByteCountFormatter stringFromByteCount:n countStyle:NSByteCountFormatterCountStyleBinary];
}

@implementation AppDelegate {
    NSStatusItem *_item;
    NSMenu *_menu;
    BOOL _menuOpen;

    NSDictionary *_status;     // the service's last status; nil if unreachable
    NSString *_serviceProblem; // why the service can't be reached
    BOOL _polling;             // a status request is running
    NSString *_busyText;       // set while a command is running
}

- (void)applicationDidFinishLaunching:(NSNotification *)notification {
    // One menu bar item is enough: quit if a copy is already running.
    NSString *bundleID = NSBundle.mainBundle.bundleIdentifier;
    pid_t me = NSProcessInfo.processInfo.processIdentifier;
    for (NSRunningApplication *app in [NSRunningApplication runningApplicationsWithBundleIdentifier:bundleID ?: @""]) {
        if (app.processIdentifier != me) {
            [NSApp terminate:nil];
            return;
        }
    }

    [self installEditMenu];

    _item = [NSStatusBar.systemStatusBar statusItemWithLength:NSSquareStatusItemLength];
    _menu = [[NSMenu alloc] init];
    _menu.delegate = self;
    _menu.autoenablesItems = NO;
    _item.menu = _menu;
    [self updateIcon];

    // Open at login unless the user has turned that off before.
    NSUserDefaults *defaults = NSUserDefaults.standardUserDefaults;
    if (![defaults boolForKey:AWGDidSetUpLoginKey]) {
        [self setOpensAtLogin:YES];
        [defaults setBool:YES forKey:AWGDidSetUpLoginKey];
    }

    [self poll];
    NSTimer *timer = [NSTimer scheduledTimerWithTimeInterval:AWGPollInterval
                                                      target:self
                                                    selector:@selector(timerFired:)
                                                    userInfo:nil
                                                     repeats:YES];
    timer.tolerance = 1;
}

// installEditMenu gives text fields copy and paste. The app shows no menu
// bar of its own, but key equivalents still go through the main menu.
- (void)installEditMenu {
    NSMenu *edit = [[NSMenu alloc] initWithTitle:@"Edit"];
    [edit addItemWithTitle:@"Cut" action:@selector(cut:) keyEquivalent:@"x"];
    [edit addItemWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"];
    [edit addItemWithTitle:@"Paste" action:@selector(paste:) keyEquivalent:@"v"];
    [edit addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@"a"];
    NSMenuItem *editItem = [[NSMenuItem alloc] init];
    editItem.submenu = edit;
    NSMenu *mainMenu = [[NSMenu alloc] init];
    [mainMenu addItem:editItem];
    NSApp.mainMenu = mainMenu;
}

#pragma mark - Talking to the service

- (void)timerFired:(NSTimer *)timer {
    [self poll];
}

- (void)poll {
    if (_polling || _busyText) {
        return;
    }
    _polling = YES;
    [self send:@{@"command" : @"status"} busyText:nil];
}

// send runs one request off the main thread, then shows the result. With
// busyText it is a user's command: the icon shows it running, and errors are
// reported.
- (void)send:(NSDictionary *)request busyText:(NSString *)busyText {
    BOOL command = busyText != nil;
    if (command) {
        _busyText = busyText;
        [self refreshUI];
    }
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED, 0), ^{
        NSError *error = nil;
        NSDictionary *reply = AWGCall(request, &error);
        dispatch_async(dispatch_get_main_queue(), ^{
            if (command) {
                self->_busyText = nil;
            } else {
                self->_polling = NO;
            }
            [self handleReply:reply error:error report:command];
        });
    });
}

- (void)handleReply:(NSDictionary *)reply error:(NSError *)error report:(BOOL)report {
    if (!reply) {
        _status = nil;
        _serviceProblem = AWGDescribeError(error);
        [self refreshUI];
        if (report) {
            [self showAlert:@"Can't reach the awg-hs service" text:_serviceProblem];
        }
        return;
    }
    _serviceProblem = nil;
    NSDictionary *status = reply[@"status"];
    if ([status isKindOfClass:[NSDictionary class]]) {
        _status = status;
    }
    [self refreshUI];
    if (report && !AWGBool(reply, @"ok")) {
        [self showAlert:@"AmneziaWG" text:AWGString(reply, @"error")];
    }
}

#pragma mark - Icon and menu

- (void)refreshUI {
    [self updateIcon];
    if (_menuOpen) {
        [self rebuildMenu];
    }
}

- (void)updateIcon {
    AWGIconState state;
    NSString *tip;
    if (_busyText) {
        state = AWGIconBusy;
        tip = _busyText;
    } else if (!_status) {
        state = AWGIconUnavailable;
        tip = _serviceProblem ?: @"AmneziaWG";
    } else if (AWGBool(_status, @"connected")) {
        state = AWGIconConnected;
        tip = [NSString stringWithFormat:@"AmneziaWG: connected to %@", AWGString(_status, @"name")];
    } else if (AWGBool(_status, @"blocking")) {
        state = AWGIconBlocked;
        tip = @"AmneziaWG: the kill switch is blocking the internet";
    } else {
        state = AWGIconDisconnected;
        tip = @"AmneziaWG: disconnected";
    }
    _item.button.image = AWGStatusIcon(state);
    _item.button.toolTip = tip;
}

- (void)menuWillOpen:(NSMenu *)menu {
    _menuOpen = YES;
    [self rebuildMenu];
    [self poll];
}

- (void)menuDidClose:(NSMenu *)menu {
    _menuOpen = NO;
}

- (void)addInfo:(NSString *)title {
    NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:title action:NULL keyEquivalent:@""];
    item.enabled = NO;
    [_menu addItem:item];
}

- (NSMenuItem *)addAction:(NSString *)title action:(SEL)action enabled:(BOOL)enabled {
    NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:title action:action keyEquivalent:@""];
    item.target = self;
    item.enabled = enabled;
    [_menu addItem:item];
    return item;
}

- (void)rebuildMenu {
    [_menu removeAllItems];
    NSDictionary *st = _status;
    BOOL connected = AWGBool(st, @"connected");
    BOOL blocking = AWGBool(st, @"blocking");

    if (_busyText) {
        [self addInfo:_busyText];
    } else if (!st) {
        [self addInfo:@"The awg-hs service can't be reached"];
        if (_serviceProblem) {
            [self addInfo:_serviceProblem];
        }
    } else if (connected) {
        [self addInfo:[NSString stringWithFormat:@"Connected to %@", AWGString(st, @"name")]];
        [self addInfo:[NSString stringWithFormat:@"Server: %@", AWGString(st, @"endpoint")]];
        [self addInfo:[self handshakeText:st]];
        [self addInfo:[NSString stringWithFormat:@"Received %@, sent %@", AWGBytes(AWGNumber(st, @"rxBytes")),
                                                 AWGBytes(AWGNumber(st, @"txBytes"))]];
        for (NSString *warning in AWGStrings(st, @"warnings")) {
            [self addInfo:[@"Warning: " stringByAppendingString:warning]];
        }
    } else if (blocking) {
        [self addInfo:@"Disconnected, and the kill switch is blocking"];
        [self addInfo:@"the internet until you reconnect or disconnect"];
    } else {
        [self addInfo:@"Disconnected"];
    }
    [_menu addItem:[NSMenuItem separatorItem]];

    BOOL ready = st != nil && !_busyText;
    if (connected) {
        [self addAction:@"Disconnect" action:@selector(disconnect:) enabled:ready];
    } else {
        NSString *saved = AWGString(st, @"savedName");
        NSString *title = saved.length ? [NSString stringWithFormat:@"Connect to %@", saved] : @"Connect";
        [self addAction:title action:@selector(connectSaved:) enabled:ready && AWGBool(st, @"hasSavedConfig")];
        if (blocking) {
            [self addAction:@"Disconnect and Lift the Kill Switch" action:@selector(disconnect:) enabled:ready];
        }
    }
    [self addAction:@"Connect with a Config File…" action:@selector(connectFile:) enabled:ready];
    [self addAction:@"Connect with a vpn:// Key…" action:@selector(connectKey:) enabled:ready];
    [_menu addItem:[NSMenuItem separatorItem]];

    NSMenuItem *killSwitch = [self addAction:@"Kill Switch" action:@selector(toggleKillSwitch:) enabled:ready];
    killSwitch.state = AWGBool(st, @"killSwitch") ? NSControlStateValueOn : NSControlStateValueOff;
    killSwitch.toolTip = @"While connected, block everything that would leave this Mac outside the tunnel.";
    NSMenuItem *login = [self addAction:@"Open at Login" action:@selector(toggleOpenAtLogin:) enabled:YES];
    login.state = [self opensAtLogin] ? NSControlStateValueOn : NSControlStateValueOff;
    [self addAction:@"Show Log" action:@selector(showLog:) enabled:YES];
    [_menu addItem:[NSMenuItem separatorItem]];
    NSMenuItem *quit = [self addAction:@"Quit AWG-HS" action:@selector(quit:) enabled:YES];
    quit.toolTip = @"Closes this menu. The VPN stays as it is.";
}

- (NSString *)handshakeText:(NSDictionary *)st {
    long long last = AWGNumber(st, @"lastHandshake");
    if (last == 0) {
        return @"No handshake yet";
    }
    long long ago = (long long)NSDate.date.timeIntervalSince1970 - last;
    if (ago < 0) {
        ago = 0;
    }
    if (ago < 120) {
        return [NSString stringWithFormat:@"Last handshake %lld s ago", ago];
    }
    return [NSString stringWithFormat:@"Last handshake %lld min ago", ago / 60];
}

#pragma mark - Actions

- (void)disconnect:(id)sender {
    [self send:@{@"command" : @"down"} busyText:@"Disconnecting…"];
}

- (void)connectSaved:(id)sender {
    [self send:@{@"command" : @"up"} busyText:@"Connecting…"];
}

- (void)connectFile:(id)sender {
    NSOpenPanel *panel = [NSOpenPanel openPanel];
    panel.title = @"Choose an AmneziaWG config";
    panel.message = @"Choose a .conf file saved from the AmneziaVPN app.";
    panel.canChooseDirectories = NO;
    panel.allowsMultipleSelection = NO;
    [NSApp activateIgnoringOtherApps:YES];
    if ([panel runModal] != NSModalResponseOK || !panel.URL) {
        return;
    }
    NSError *error = nil;
    NSString *text = [NSString stringWithContentsOfURL:panel.URL encoding:NSUTF8StringEncoding error:&error];
    if (!text) {
        [self showAlert:@"The file can't be read" text:error.localizedDescription];
        return;
    }
    NSString *name = panel.URL.lastPathComponent.stringByDeletingPathExtension;
    [self send:@{@"command" : @"up", @"config" : text, @"name" : name}
        busyText:[NSString stringWithFormat:@"Connecting to %@…", name]];
}

- (void)connectKey:(id)sender {
    NSAlert *alert = [[NSAlert alloc] init];
    alert.messageText = @"Connect with a vpn:// key";
    alert.informativeText = @"Paste the key for your self-hosted AmneziaWG server, copied from the AmneziaVPN app.";
    [alert addButtonWithTitle:@"Connect"];
    [alert addButtonWithTitle:@"Cancel"];
    NSTextField *field = [[NSTextField alloc] initWithFrame:NSMakeRect(0, 0, 380, 24)];
    field.placeholderString = @"vpn://…";
    alert.accessoryView = field;
    alert.window.initialFirstResponder = field;
    [NSApp activateIgnoringOtherApps:YES];
    if ([alert runModal] != NSAlertFirstButtonReturn) {
        return;
    }
    NSString *key = [field.stringValue stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet];
    if (![key hasPrefix:@"vpn://"]) {
        [self showAlert:@"That isn't a vpn:// key" text:@"A key starts with vpn:// followed by a long run of letters and digits."];
        return;
    }
    [self send:@{@"command" : @"up", @"config" : key, @"name" : @"vpn key"} busyText:@"Connecting…"];
}

- (void)toggleKillSwitch:(NSMenuItem *)sender {
    BOOL on = sender.state != NSControlStateValueOn;
    [self send:@{@"command" : @"killswitch", @"enable" : on ? @YES : @NO}
        busyText:on ? @"Turning on the kill switch…" : @"Turning off the kill switch…"];
}

- (void)toggleOpenAtLogin:(NSMenuItem *)sender {
    [self setOpensAtLogin:sender.state != NSControlStateValueOn];
}

- (void)showLog:(id)sender {
    if (![NSWorkspace.sharedWorkspace openFile:AWGLogPath withApplication:@"Console"]) {
        [NSWorkspace.sharedWorkspace openURL:[NSURL fileURLWithPath:AWGLogPath]];
    }
}

- (void)quit:(id)sender {
    [NSApp terminate:nil];
}

- (void)showAlert:(NSString *)title text:(NSString *)text {
    NSAlert *alert = [[NSAlert alloc] init];
    alert.messageText = title;
    alert.informativeText = text.length ? text : @"";
    [NSApp activateIgnoringOtherApps:YES];
    [alert runModal];
}

#pragma mark - Open at login

// Opening at login uses a per-user LaunchAgent, which works on every macOS
// this app supports and needs no helper app.

- (NSString *)loginAgentPath {
    NSString *name = [AWGLoginAgentLabel stringByAppendingPathExtension:@"plist"];
    return [[NSHomeDirectory() stringByAppendingPathComponent:@"Library/LaunchAgents"] stringByAppendingPathComponent:name];
}

- (BOOL)opensAtLogin {
    return [NSFileManager.defaultManager fileExistsAtPath:[self loginAgentPath]];
}

- (void)setOpensAtLogin:(BOOL)on {
    NSString *path = [self loginAgentPath];
    NSError *error = nil;
    if (!on) {
        if (![NSFileManager.defaultManager removeItemAtPath:path error:&error] && [self opensAtLogin]) {
            [self showAlert:@"Couldn't turn off Open at Login" text:error.localizedDescription];
        }
        return;
    }
    NSDictionary *agent = @{
        @"Label" : AWGLoginAgentLabel,
        @"ProgramArguments" : @[ @"/usr/bin/open", @"-a", NSBundle.mainBundle.bundlePath ],
        @"RunAtLoad" : @YES,
        @"LimitLoadToSessionType" : @"Aqua",
    };
    [NSFileManager.defaultManager createDirectoryAtPath:path.stringByDeletingLastPathComponent
                            withIntermediateDirectories:YES
                                             attributes:nil
                                                  error:NULL];
    if (![agent writeToFile:path atomically:YES]) {
        [self showAlert:@"Couldn't turn on Open at Login" text:path];
    }
}

@end
