//go:build windows

// One-time admin setup: a Task Scheduler entry lets PhoneLock start with admin
// rights afterwards without a UAC prompt. Nothing here starts PhoneLock with Windows.
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const taskName = "PhoneLock"

func isAdmin() bool {
	r, _, _ := procIsUserAnAdmin.Call()
	return r != 0
}

func hiddenCmd(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	return c
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// taskState: 1 = task launches this exact exe, 0 = missing or points elsewhere, -1 = couldn't check.
func taskState(exe string) int {
	if hiddenCmd("schtasks", "/query", "/tn", taskName).Run() != nil {
		return 0
	}
	out, err := hiddenCmd("schtasks", "/query", "/tn", taskName, "/xml").Output()
	if err != nil {
		return -1
	}
	text := string(out)
	if len(out) >= 2 && out[0] == 0xFF && out[1] == 0xFE { // UTF-16 output
		u := make([]uint16, (len(out)-2)/2)
		binary.Read(bytes.NewReader(out[2:]), binary.LittleEndian, &u)
		text = string(utf16.Decode(u))
	}
	text = strings.ToLower(text)
	if strings.Contains(text, strings.ToLower(exe)) || strings.Contains(text, strings.ToLower(xmlEscape(exe))) {
		return 1
	}
	return 0
}

func taskPointsTo(exe string) bool { return taskState(exe) == 1 }

// ensureSetup runs while elevated: registers the no-prompt task and the firewall rule.
func ensureSetup(exe string) error {
	if taskPointsTo(exe) {
		return nil
	}
	u, err := user.Current()
	if err != nil {
		return err
	}
	taskXML := `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>Lets PhoneLock open with admin rights without a UAC prompt. It only runs when you open PhoneLock yourself.</Description>
  </RegistrationInfo>
  <Principals>
    <Principal id="Author">
      <UserId>` + xmlEscape(u.Username) + `</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>false</StartWhenAvailable>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>4</Priority>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>"` + xmlEscape(exe) + `"</Command>
      <WorkingDirectory>` + xmlEscape(filepath.Dir(exe)) + `</WorkingDirectory>
    </Exec>
  </Actions>
</Task>`
	// schtasks wants UTF-16 LE with a BOM.
	enc := utf16.Encode([]rune(taskXML))
	var buf bytes.Buffer
	buf.Write([]byte{0xFF, 0xFE})
	binary.Write(&buf, binary.LittleEndian, enc)
	tmp := filepath.Join(os.TempDir(), "phonelock-task.xml")
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return err
	}
	defer os.Remove(tmp)
	if out, err := hiddenCmd("schtasks", "/create", "/tn", taskName, "/xml", tmp, "/f").CombinedOutput(); err != nil {
		return &setupErr{"Couldn't create the PhoneLock task: " + strings.TrimSpace(string(out))}
	}

	// Allow the phone to reach PhoneLock through Windows Firewall.
	hiddenCmd("netsh", "advfirewall", "firewall", "delete", "rule", "name="+taskName).Run()
	hiddenCmd("netsh", "advfirewall", "firewall", "add", "rule", "name="+taskName, "dir=in", "action=allow",
		"program="+exe, "enable=yes", "profile=any").Run()
	return nil
}

type setupErr struct{ s string }

func (e *setupErr) Error() string { return e.s }

// launchElevated is called when PhoneLock was opened without admin rights.
// Returns after handing off to an elevated copy (or failing).
func launchElevated(exe string) {
	if taskState(exe) != 0 {
		if hiddenCmd("schtasks", "/run", "/tn", taskName).Run() == nil {
			return
		}
	}
	// First run (or the exe moved): ask for admin once.
	r, _, _ := procShellExecuteW.Call(0, uintptr(unsafe.Pointer(u16("runas"))), uintptr(unsafe.Pointer(u16(exe))),
		uintptr(unsafe.Pointer(u16("--setup"))), uintptr(unsafe.Pointer(u16(filepath.Dir(exe)))), 1)
	if r <= 32 {
		messageBox("PhoneLock needs admin rights once to finish setting up, so it can block input in every window.\n\nOpen PhoneLock again and click Yes on the prompt. You won't be asked again after that.", mbIconInfo)
	}
}
