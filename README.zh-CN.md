# Simple Timer

[English](README.md)

一个简单的 Windows 30 秒倒计时小工具，用来提醒 MapleStory 中 Fatal Stricke 的 CD 时间。

程序是 64x64 透明无边框窗口，支持置顶、拖拽、右下角缩放和全局快捷键；不注入游戏、不读内存、不模拟输入。

程序只允许同时运行一个实例。如果 `Simple Timer.exe` 已经在运行，再次启动会提示并退出。

`timer.png` 会在构建时被嵌入到可执行文件里。程序会以 70% 透明度显示这张图片，并覆盖一层顺时针逐秒减少的阴影。`salute.jpg` 会被转换成程序图标。

## 快捷键

- 默认快捷键：
	- `Alt+F8`：启动 / 继续
	- `Alt+F6`：暂停
	- `Alt+F7`：重置到 30 秒
- 你可以通过 EXE 同目录的 `config.json` 自定义快捷键。
- 也可以右键计时器选择 `Settings`，点选快捷键输入框后直接按组合键来设置。
- 如果该文件不存在，程序会自动按默认值创建。

示例：

```json
{
	"hotkeys": {
		"start": "Alt+F8",
		"pause": "Alt+F6",
		"reset": "Ctrl+Shift+R"
	},
	"window": {
		"x": 0,
		"y": 0,
		"width": 64,
		"height": 64,
		"saved": false
	}
}
```

支持格式：

- 不区分大小写，使用 `+` 连接，例如 `Ctrl+Alt+F9`
- 修饰键：`Alt`、`Ctrl`/`Control`、`Shift`、`Win`
- 主键：`A-Z`、`0-9`、`F1-F24`、`Space`、`Tab`、`Enter`、`Esc`
- 某个快捷键配置无效时，只会该项回退到默认值
- 程序内快捷键录入框使用 Win32 hotkey control，可录入常规 `Ctrl` / `Alt` / `Shift` 组合；如果要使用 `Win` 组合，请手动编辑 `config.json`。

- 鼠标左键拖拽移动位置
- 右下角拖拽调整窗口大小
- 鼠标右键打开菜单，可选择 `Settings` 或 `Exit`

## 设置菜单

- 右键计时器窗口，选择 `Settings`，点选快捷键输入框后按下想设置的组合键。
- 保存后会写入 `config.json`，并立即重新注册热键。
- 在同一个菜单里选择 `Exit` 可以关闭程序。

## 位置保存

- 拖拽或缩放窗口后，会保存位置和大小到 EXE 同目录的 `config.json`。
- 下次打开 `Simple Timer.exe` 会自动恢复到上一次位置和大小。
- 如果删除 `config.json`，程序会按默认值重新创建，并回到默认初始位置。

## 构建

```powershell
go run ./tools/makeicon -in "salute.jpg" -out "app.ico"
windres -O coff -F pe-x86-64 -i "resource.rc" -o "rsrc_windows_amd64.syso"
go build -ldflags="-H windowsgui" -o "Simple Timer.exe" .
```

构建完成后，`Simple Timer.exe` 可以单独运行。只有需要更换计时器图片并重新构建时，才需要 `timer.png`。

## 安全边界

本程序只使用普通 Win32 窗口、透明分层窗口和 `RegisterHotKey` 全局热键；没有使用 AutoHotkey、键鼠 hook、`SendInput`、模拟按键、读内存、DLL 注入或 DirectX 注入式 overlay。

本程序不会读取、修改、检测或控制 MapleStory / Nexon / 其他任何程序，也不会向其他程序发送键盘、鼠标或宏输入。

单实例保护只使用本程序自己的 Win32 命名互斥量，不枚举进程，也不检查其他窗口。

## 免责声明

本工具仅用于个人倒计时提醒，不是 Nexon、Wizet 或 MapleStory 官方工具，也不与其存在任何关联。

没有任何同机运行的程序可以承诺 100% 不触发游戏反作弊误判。使用本工具的风险由使用者自行承担；如果账号安全优先，第二屏计时器仍然是最稳妥方案。
