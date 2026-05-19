# Simple Timer

[English](README.md)

一个简单的 Windows 30 秒倒计时小工具，用来提醒 MapleStory 中 Fatal Stricke 的 CD 时间。

程序是 40x40 透明无边框窗口，支持置顶、拖拽和全局快捷键；不注入游戏、不读内存、不模拟输入。

`timer.png` 会在构建时被嵌入到可执行文件里。程序会以 70% 透明度显示这张图片，并覆盖一层顺时针逐秒减少的阴影。`salute.jpg` 会被转换成程序图标。

## 快捷键

- `Alt+F8`：启动 / 继续
- `Alt+F6`：暂停
- `Alt+F7`：重置到 30 秒
- 鼠标左键拖拽移动位置
- 鼠标右键关闭程序

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

## 免责声明

本工具仅用于个人倒计时提醒，不是 Nexon、Wizet 或 MapleStory 官方工具，也不与其存在任何关联。

没有任何同机运行的程序可以承诺 100% 不触发游戏反作弊误判。使用本工具的风险由使用者自行承担；如果账号安全优先，第二屏计时器仍然是最稳妥方案。
