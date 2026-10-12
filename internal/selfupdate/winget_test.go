package selfupdate

import "testing"

func TestIsWingetPath(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		// Where winget keeps the file, and the link it puts on the PATH.
		{`C:\Users\someone\AppData\Local\Microsoft\WinGet\Packages\sevenam.gitraffe_Microsoft.Winget.Source_8wekyb3d8bbwe\gitraffe.exe`, true},
		{`C:\Users\someone\AppData\Local\Microsoft\WinGet\Links\gitraffe.exe`, true},

		// An install for every user of the machine.
		{`C:\Program Files\WinGet\Packages\sevenam.gitraffe_Microsoft.Winget.Source_8wekyb3d8bbwe\gitraffe.exe`, true},
		{`C:\Program Files\WinGet\Links\gitraffe.exe`, true},

		// Windows paths are not case sensitive, and may arrive with either slash.
		{`c:\users\someone\appdata\local\microsoft\winget\links\gitraffe.exe`, true},
		{`C:/Users/someone/AppData/Local/Microsoft/WinGet/Links/gitraffe.exe`, true},

		// Anywhere else is gitraffe's own to replace.
		{`C:\Users\someone\go\bin\gitraffe.exe`, false},
		{`D:\git\sevenam\gitraffe\gitraffe.exe`, false},
		{`C:\tools\winget\gitraffe.exe`, false},
		{`/usr/local/bin/gitraffe`, false},
		{``, false},
	} {
		if got := isWingetPath(tc.path); got != tc.want {
			t.Errorf("isWingetPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestInstallRefusesAWingetCopy(t *testing.T) {
	was := ManagedByWinget
	ManagedByWinget = func() bool { return true }
	t.Cleanup(func() { ManagedByWinget = was })

	// A release with no assets: reaching the download would fail differently.
	if err := Install(&Release{TagName: "v9.9.9"}); err != ErrManagedByWinget {
		t.Errorf("Install = %v, want ErrManagedByWinget", err)
	}
}
