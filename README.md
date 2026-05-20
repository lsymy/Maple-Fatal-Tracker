# Simple Timer

<img src="preview.png" alt="Simple Timer preview" width="72">

[中文说明](README.zh-CN.md)

A simple 30-second Windows countdown timer for reminding the Fatal Stricke cooldown in MapleStory.

The app is a 64x64 transparent borderless window with always-on-top, drag-and-drop positioning, corner resizing, and global hotkeys. It does not inject into the game, read process memory, or simulate input.

`timer.png` is embedded into the executable at build time. The app displays it at 70% opacity and overlays a smooth clockwise shadow countdown. `salute.jpg` is converted into the application icon.

## Hotkeys

- By default:
	- `Alt+F8`: Start / resume
	- `Alt+F6`: Pause
	- `Alt+F7`: Reset to 30 seconds
- You can customize hotkeys with `simple-timer-hotkeys.json` in the same folder as the EXE.
- If the file does not exist, the app creates it automatically with defaults.

Example:

```json
{
	"start": "Alt+F8",
	"pause": "Alt+F6",
	"reset": "Ctrl+Shift+R"
}
```

Supported format:

- Case-insensitive, using `+` to join keys, e.g. `Ctrl+Alt+F9`
- Modifiers: `Alt`, `Ctrl`/`Control`, `Shift`, `Win`
- Main key: `A-Z`, `0-9`, `F1-F24`, `Space`, `Tab`, `Enter`, `Esc`
- If one hotkey is invalid, only that action falls back to its default value

- Left mouse button: Drag the timer window
- Bottom-right corner: Drag to resize the timer window
- Right mouse button: Close the app

## Position Persistence

- After dragging or resizing the window, the position and size are saved to `simple-timer-position.json` next to the executable.
- The next time `Simple Timer.exe` starts, it automatically restores the last saved position and size.
- If you delete `simple-timer-position.json`, the app returns to the default initial position.

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
