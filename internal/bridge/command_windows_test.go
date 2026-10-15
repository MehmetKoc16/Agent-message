package bridge

import (
	"strings"
	"testing"
)

func Test_windows_komut_satiri_siniri_kacis_ve_tum_alanlari_kapsar(t *testing.T) {
	// NEDEN: tam sınır kabul edilmeli; exe boşlukları ve prompt tırnakları da yer tutar.
	// "agy.exe -p " = 11 birim; 31989 birim düz prompt tam 32000 eder.
	prompt := strings.Repeat("a", 31989)
	if !agyCommandFits("agy.exe", []string{"-p", prompt}) {
		t.Fatal("tam sınır reddedildi")
	}
	if agyCommandFits("agy.exe", []string{"-p", prompt + "a"}) {
		t.Fatal("sınırın bir fazlası kabul edildi")
	}
	if agyCommandFits("agy.exe", []string{"-p", prompt, "--model", "m"}) {
		t.Fatal("ek argümanlar sayılmadı")
	}
	if agyCommandFits("a y.exe", []string{"-p", prompt}) {
		t.Fatal("exe tırnakları sayılmadı")
	}
	if agyCommandFits("agy.exe", []string{"-p", strings.Repeat(`"`, 16000)}) {
		t.Fatal("prompt kaçışları sayılmadı")
	}
}
