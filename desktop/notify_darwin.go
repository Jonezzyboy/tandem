package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework UserNotifications
#import <UserNotifications/UserNotifications.h>
#include <stdlib.h>

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

static int notificationDelivered(const char *ident) {
	NSString *want = [NSString stringWithUTF8String:ident];
	__block int found = 0;
	dispatch_semaphore_t done = dispatch_semaphore_create(0);
	[[UNUserNotificationCenter currentNotificationCenter] getDeliveredNotificationsWithCompletionHandler:^(NSArray<UNNotification *> *list) {
		for (UNNotification *n in list) {
			if ([n.request.identifier isEqualToString:want]) found = 1;
		}
		dispatch_semaphore_signal(done);
	}];
	dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, 10 * NSEC_PER_SEC));
	return found;
}
*/
import "C"

import "unsafe"

func notificationsAllowed() bool {
	return C.notificationsAllowed() == 1
}

func notificationDelivered(id string) bool {
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))
	return C.notificationDelivered(cid) == 1
}
