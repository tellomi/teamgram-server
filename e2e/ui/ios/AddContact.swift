import Contacts
import Foundation

// Best-effort contact insert inside the iOS Simulator. A CLI has no
// Info.plist, so Contacts permission is often denied; ios_contacts.py is the
// fallback that writes AddressBook.sqlitedb.

let args = CommandLine.arguments
guard args.count == 3 else {
    fputs("usage: AddContact NAME +PHONE\n", stderr)
    exit(2)
}

let name = args[1]
let phone = args[2]
let store = CNContactStore()
let wait = DispatchSemaphore(value: 0)
var allowed = false
store.requestAccess(for: .contacts) { granted, error in
    if let error {
        fputs("contacts access: \(error.localizedDescription)\n", stderr)
    }
    allowed = granted
    wait.signal()
}
wait.wait()
if !allowed {
    fputs("contacts access denied\n", stderr)
    exit(2)
}

let contact = CNMutableContact()
contact.givenName = name
let number = CNPhoneNumber(stringValue: phone)
contact.phoneNumbers = [CNLabeledValue(label: CNLabelPhoneNumberMobile, value: number)]
let request = CNSaveRequest()
request.add(contact, toContainerWithIdentifier: nil)
do {
    try store.execute(request)
    print("ok \(name)")
} catch {
    fputs("save failed: \(error.localizedDescription)\n", stderr)
    exit(1)
}
