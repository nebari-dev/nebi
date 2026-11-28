package jobs

import "golang.org/x/sys/windows/registry"

// readMachineID returns the MachineGuid Windows sets at install.
func readMachineID() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return ""
	}
	defer k.Close()
	id, _, err := k.GetStringValue("MachineGuid")
	if err != nil {
		return ""
	}
	return id
}
