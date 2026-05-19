# Simple Timer

<img src="preview.png" alt="Simple Timer preview" width="72">

[中文说明](README.zh-CN.md)

A simple 30-second Windows countdown timer for reminding the Fatal Stricke cooldown in MapleStory.

The app is a 40x40 transparent borderless window with always-on-top, drag-and-drop positioning, and global hotkeys. It does not inject into the game, read process memory, or simulate input.

`timer.png` is embedded into the executable at build time. The app displays it at 70% opacity and overlays a smooth clockwise shadow countdown. `salute.jpg` is converted into the application icon.

## Hotkeys

- `Alt+F8`: Start / resume
- `Alt+F6`: Pause
- `Alt+F7`: Reset to 30 seconds
- Left mouse button: Drag the timer window
- Right mouse button: Close the app

## Build

```powershell
go run ./tools/makeicon -in "salute.jpg" -out "app.ico"
windres -O coff -F pe-x86-64 -i "resource.rc" -o "rsrc_windows_amd64.syso"
go build -ldflags="-H windowsgui" -o "Simple Timer.exe" .
```

After building, `Simple Timer.exe` can run by itself. `timer.png` is only needed when rebuilding with a different timer image.

## Safety Boundary

This app only uses a normal Win32 window, layered-window rendering, and `RegisterHotKey` for global hotkeys. It does not use AutoHotkey, keyboard or mouse hooks, `SendInput`, simulated key presses, process memory reading, DLL injection, or DirectX overlay injection.

This app does not read, modify, detect, or control MapleStory / Nexon / any other program. It also does not send keyboard, mouse, or macro input to other programs.

## Disclaimer

This tool is only for personal countdown reminders. It is not an official Nexon, Wizet, or MapleStory tool, and it is not affiliated with them in any way.

No program running on the same PC can guarantee that it will never be flagged by game anti-cheat systems. Use this tool at your own risk. If account safety is the top priority, a timer on a second screen or separate device is still the safest option.
