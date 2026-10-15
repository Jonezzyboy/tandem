package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework UserNotifications
#import <UserNotifications/UserNotifications.h>

// notificationsAllowed is 1 when macOS will show the app's notifications, 0
// when it won't, and -1 when it didn't answer. Turning notifications on in
// System Settings after refusing the prompt enables alerts but can leave
// authorizationStatus at denied, so alertSetting counts too.
static int notificationsAllowed(void) {
	__block int allowed = -1;
	dispatch_semaphore_t done = dispatch_semaphore_create(0);
	[[UNUserNotificationCenter currentNotificationCenter] getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *s) {
		UNAuthorizationStatus st = s.authorizationStatus;
		allowed = st == UNAuthorizationStatusAuthorized || st == UNAuthorizationStatusProvisional ||
			s.alertSetting == UNNotificationSettingEnabled;
		dispatch_semaphore_signal(done);
	}];
	dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, 10 * NSEC_PER_SEC));
	return allowed;
}
*/
import "C"

func notificationsAllowed() bool {
	return C.notificationsAllowed() == 1
}
