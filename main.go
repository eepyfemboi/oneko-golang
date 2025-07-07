//go:build windows

package main

import (
	_ "embed"
	_ "image/png"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows/registry"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/getlantern/systray"
)

//go:embed oneko.png
var onekoData []byte

//go:embed oneko.ico
var trayIcon []byte

const startupKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const appName = "Oneko"

func isStartupEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, startupKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	_, _, err = k.GetStringValue(appName)
	return err == nil
}

func toggleStartup() {
	exePath, _ := os.Executable()
	exePath, _ = filepath.Abs(exePath)

	if isStartupEnabled() {
		k, err := registry.OpenKey(registry.CURRENT_USER, startupKey, registry.SET_VALUE)
		if err == nil {
			_ = k.DeleteValue(appName)
			_ = k.Close()
		}
	} else {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, startupKey, registry.SET_VALUE)
		if err == nil {
			_ = k.SetStringValue(appName, exePath)
			_ = k.Close()
		}
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	_ = cmd.Start()
}

func onReady() {
	systray.SetIcon(trayIcon)
	systray.SetTitle("Oneko (By Sleepy :3)")
	systray.SetTooltip("Details")

	mStartup := systray.AddMenuItemCheckbox("Auto-Run", "Automatically run Oneko when you log in :3", isStartupEnabled())
	mVisit := systray.AddMenuItem("Website", "Open the website for this project :3")
	mQuit := systray.AddMenuItem("Close", "Close oneko")

	go func() {
		for {
			select {
			case <-mStartup.ClickedCh:
				toggleStartup()
				mStartup.Check()
				if !isStartupEnabled() {
					mStartup.Uncheck()
				}
			case <-mVisit.ClickedCh:
				openBrowser("https://sleepie.dev/oneko")
			case <-mQuit.ClickedCh:
				systray.Quit()
				os.Exit(0)
			}
		}
	}()
}

func onExit() {}

type POINT struct{ X, Y int32 }

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	getCursorPos         = user32.NewProc("GetCursorPos")
	getWindowLong        = user32.NewProc("GetWindowLongW")
	setWindowLong        = user32.NewProc("SetWindowLongW")
	setLayeredAttr       = user32.NewProc("SetLayeredWindowAttributes")
	setWindowPos         = user32.NewProc("SetWindowPos")
	GWL_EXSTYLE    int32 = -20
	HWND_TOPMOST   int32 = -1
)

func GetCursorPos(p *POINT) {
	getCursorPos.Call(uintptr(unsafe.Pointer(p)))
}

func makeWindowTransparent(hwnd syscall.Handle) {
	const (
		GWL_STYLE         = -16
		GWL_EXSTYLE       = -20
		WS_POPUP          = 0x80000000
		WS_VISIBLE        = 0x10000000
		WS_EX_APPWINDOW   = 0x00040000
		WS_EX_LAYERED     = 0x80000
		WS_EX_TRANSPARENT = 0x20
		WS_EX_TOOLWINDOW  = 0x00000080
	)

	exStyle, _, _ := getWindowLong.Call(uintptr(hwnd), uintptr(^uintptr(19)))

	exStyle &^= WS_EX_APPWINDOW
	exStyle |= WS_EX_LAYERED | WS_EX_TRANSPARENT | WS_EX_TOOLWINDOW

	setWindowLong.Call(uintptr(hwnd), uintptr(^uintptr(15)), uintptr(WS_POPUP|WS_VISIBLE))

	setWindowLong.Call(uintptr(hwnd), uintptr(^uintptr(19)), exStyle)
	setLayeredAttr.Call(uintptr(hwnd), 0xFF00FF, 0, 0x01)
}

func setAlwaysOnTop(hwnd syscall.Handle) {
	setWindowPos.Call(uintptr(hwnd), uintptr(HWND_TOPMOST), 0, 0, 0, 0, 0x0001|0x0002)
}

var (
	spriteSize   = 32
	spriteSize_1 = float32(spriteSize)
	nekoSpeed    = float32(10)
	idleTime     = 0
	idleFrame    = 0
	idleAnim     = ""
	frameCount   = 0
	lastTick     = time.Now()
	nekoPos      = rl.Vector2{X: 100, Y: 100}
	mousePos     = rl.Vector2{}
	spriteSheet  rl.Texture2D
)

var spriteMap = map[string][]rl.Rectangle{
	"idle":         {frame(-3, -3)},
	"alert":        {frame(-7, -3)},
	"tired":        {frame(-3, -2)},
	"sleeping":     {frame(-2, 0), frame(-2, -1)},
	"scratchSelf":  {frame(-5, 0), frame(-6, 0), frame(-7, 0)},
	"scratchWallN": {frame(0, 0), frame(0, -1)},
	"scratchWallS": {frame(-7, -1), frame(-6, -2)},
	"scratchWallE": {frame(-2, -2), frame(-2, -3)},
	"scratchWallW": {frame(-4, 0), frame(-4, -1)},
	"N":            {frame(-1, -2), frame(-1, -3)},
	"NE":           {frame(0, -2), frame(0, -3)},
	"E":            {frame(-3, 0), frame(-3, -1)},
	"SE":           {frame(-5, -1), frame(-5, -2)},
	"S":            {frame(-6, -3), frame(-7, -2)},
	"SW":           {frame(-5, -3), frame(-6, -1)},
	"W":            {frame(-4, -2), frame(-4, -3)},
	"NW":           {frame(-1, 0), frame(-1, -1)},
}

func loadSprite() {
	img := rl.LoadImageFromMemory(".png", onekoData, int32(len(onekoData)))
	if img == nil {
		panic("failed to load embedded image")
	}
	spriteSheet = rl.LoadTextureFromImage(img)
	rl.UnloadImage(img)
}

func frame(x, y int) rl.Rectangle {
	return rl.NewRectangle(float32(-x*spriteSize), float32(-y*spriteSize), spriteSize_1, spriteSize_1)
}

func getDirection(dx, dy float32) string {
	dir := ""
	if dy > 0.5 {
		dir += "S"
	} else if dy < -0.5 {
		dir += "N"
	}
	if dx > 0.5 {
		dir += "E"
	} else if dx < -0.5 {
		dir += "W"
	}
	if dir == "" {
		return "idle"
	}
	return dir
}

func resetIdle() {
	idleAnim = ""
	idleFrame = 0
}

func tryIdleAnimation() {
	idleTime++
	if idleTime > 10 && idleAnim == "" && rand.Intn(200) == 0 {
		options := []string{"sleeping", "scratchSelf"}
		if nekoPos.X < 32 {
			options = append(options, "scratchWallW")
		}
		if nekoPos.Y < 32 {
			options = append(options, "scratchWallN")
		}
		if nekoPos.X > float32(rl.GetScreenWidth()-32) {
			options = append(options, "scratchWallE")
		}
		if nekoPos.Y > float32(rl.GetScreenHeight()-32) {
			options = append(options, "scratchWallS")
		}
		idleAnim = options[rand.Intn(len(options))]
		idleFrame = 0
	}
}

func updateLogic() {
	var cursor POINT
	GetCursorPos(&cursor)
	mousePos = rl.Vector2{X: float32(cursor.X), Y: float32(cursor.Y)}

	dx := mousePos.X - nekoPos.X
	dy := mousePos.Y - nekoPos.Y
	dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

	state := ""

	if dist >= 48 {
		if idleAnim != "" || idleTime > 0 {
			resetIdle()
		}

		nx := dx / dist
		ny := dy / dist
		nekoPos.X += nx * nekoSpeed
		nekoPos.Y += ny * nekoSpeed
		state = getDirection(nx, ny)
	} else {
		tryIdleAnimation()

		switch idleAnim {
		case "sleeping":
			if idleFrame < 8 {
				state = "tired"
			} else {
				state = "sleeping"
			}
			if idleFrame > 192 {
				resetIdle()
			}
		case "scratchSelf", "scratchWallN", "scratchWallS", "scratchWallE", "scratchWallW":
			state = idleAnim
			if idleFrame > 9 {
				resetIdle()
			}
		default:
			state = "idle"
		}
		idleFrame++
	}

	rl.SetWindowPosition(int(nekoPos.X)-16, int(nekoPos.Y)-16)

	rl.BeginDrawing()
	rl.ClearBackground(rl.NewColor(255, 0, 255, 255))

	frames := spriteMap[state]
	index := frameCount % len(frames)
	rl.DrawTextureRec(spriteSheet, frames[index], rl.Vector2{}, rl.White)

	rl.EndDrawing()
	frameCount++
}

func runOneko() {
	rand.Seed(time.Now().UnixNano())
	rl.InitWindow(32, 32, "")
	rl.SetWindowState(rl.FlagWindowUndecorated | rl.FlagWindowTransparent | rl.FlagWindowAlwaysRun)
	//spriteSheet = rl.LoadTexture("oneko.png")
	loadSprite()

	hwnd := syscall.Handle(rl.GetWindowHandle())
	makeWindowTransparent(hwnd)
	setAlwaysOnTop(hwnd)

	rl.SetTargetFPS(60)

	for !rl.WindowShouldClose() {
		if time.Since(lastTick) >= 100*time.Millisecond {
			lastTick = time.Now()
			updateLogic()
		} else {
			time.Sleep(5 * time.Millisecond)
		}
	}

	rl.UnloadTexture(spriteSheet)
	rl.CloseWindow()
}

func main() {
	go systray.Run(onReady, onExit)
	runOneko()
}
