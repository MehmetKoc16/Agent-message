//go:build windows

package client

import "syscall"

// SetupConsole konsol kod sayfasını UTF-8 (65001) yapar ve dönen işlevle eski haline getirir.
// Çıktı dosyaya/boruya yönlendirilmişse (konsol yoksa) çağrılar sessizce etkisizdir.
func SetupConsole() func() {
	k := syscall.NewLazyDLL("kernel32.dll")
	getOut, setOut := k.NewProc("GetConsoleOutputCP"), k.NewProc("SetConsoleOutputCP")
	getIn, setIn := k.NewProc("GetConsoleCP"), k.NewProc("SetConsoleCP")
	oldOut, _, _ := getOut.Call()
	oldIn, _, _ := getIn.Call()
	if oldOut != 0 {
		setOut.Call(65001)
	}
	if oldIn != 0 {
		setIn.Call(65001)
	}
	return func() {
		if oldOut != 0 {
			setOut.Call(oldOut)
		}
		if oldIn != 0 {
			setIn.Call(oldIn)
		}
	}
}
