//go:build darwin

package platform

import "testing"

func TestParseDarwinBatteryPercentage(t *testing.T) {
	input := `Now drawing from 'Battery Power'
 -InternalBattery-0 (id=1234567)	20%; discharging; 1:25 remaining present: true`

	got, err := parseDarwinBatteryPercentage(input)
	if err != nil {
		t.Fatalf("parseDarwinBatteryPercentage() error = %v", err)
	}
	if got != 20 {
		t.Fatalf("parseDarwinBatteryPercentage() = %d, want 20", got)
	}
}

func TestParseDarwinBatteryPercentageRejectsMissingValue(t *testing.T) {
	if _, err := parseDarwinBatteryPercentage("No batteries are currently available."); err == nil {
		t.Fatal("parseDarwinBatteryPercentage() expected error")
	}
}

func TestDarwinOnExternalPower(t *testing.T) {
	for out, want := range map[string]bool{
		"Now drawing from 'AC Power'\n -InternalBattery-0 (id=1)\t18%; charging; 2:01 remaining present: true":  true,
		"Now drawing from 'AC Power'\n -InternalBattery-0 (id=1)\t80%; AC attached; not charging present: true": true,
		"Now drawing from 'Battery Power'\n -InternalBattery-0 (id=1)\t20%; discharging; 1:25 remaining":        false,
	} {
		if got := darwinOnExternalPower(out); got != want {
			t.Errorf("darwinOnExternalPower(%q) = %v", out, got)
		}
	}
}
