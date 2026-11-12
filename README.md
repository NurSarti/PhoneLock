# PhoneLock

Press **Alt + Shift + Win** on your PC to lock the keyboard and mouse. Tap the button in the phone app to unlock (or lock) it. The screen keeps working normally while it's locked.

## Get it

Download both files from the latest [Release](../../releases/latest):

- **PhoneLock.exe** for the PC
- **PhoneLock.apk** for the phone (Android 8+)

## PC

1. Put `PhoneLock.exe` somewhere permanent, then open it.
2. The first time, Windows asks for admin permission. Click **Yes**. That's the only time it asks.
3. A padlock appears in the system tray (bottom-right, maybe under the `^` arrow). Green means unlocked, red means locked.
4. Right-click the padlock to see the PC's address, lock it, or quit.

PhoneLock always starts **unlocked** and does **not** start with Windows. If you're ever stuck, restart the PC, or press Ctrl + Alt + Del and sign out or restart from there.

## Phone

1. Install `PhoneLock.apk`. If Samsung asks, allow **Install unknown apps**. If Play Protect warns, tap **Install anyway**.
2. Be on the same Wi-Fi as the PC. The app finds the PC by itself.
3. Tap the note to edit it. Tap the big button to lock or unlock.

If the app can't find the PC, tap the gear and type the address shown in the PC's tray menu.

## How it works

- PC app: Go, Windows low-level keyboard/mouse hooks, HTTP on port 47811, UDP discovery on 47812.
- Phone app: plain Android (Java).
- Both apps share the key in `shared/key.txt`. GitHub Actions builds both and publishes a Release on every push to `main`.
