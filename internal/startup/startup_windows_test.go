package startup

import "testing"

func TestPlatformStartupWindowsCommand(t *testing.T) {
	r, err := New(`C:\Program Files\司南\sinan.exe`)
	if err != nil {
		t.Fatal(err)
	}
	if r.(windowsRegistration).command != `"C:\Program Files\司南\sinan.exe"` {
		t.Fatal("executable must be quoted")
	}
}
