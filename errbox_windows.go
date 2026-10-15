package main

import "golang.org/x/sys/windows"

func showError(msg string) {
	t, _ := windows.UTF16PtrFromString("Contest Dashboard")
	m, _ := windows.UTF16PtrFromString(msg)
	windows.MessageBox(0, m, t, windows.MB_OK|windows.MB_ICONERROR)
}
