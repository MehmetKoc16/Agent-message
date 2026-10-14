// sha256, sha256sum olmayan sistemler için dağıtım özeti üretir.
package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "kullanım: sha256 <dizin> <dosya>...")
		os.Exit(1)
	}
	for _, name := range os.Args[2:] {
		f, err := os.Open(filepath.Join(os.Args[1], name))
		if err != nil {
			fmt.Fprintln(os.Stderr, "özet için dosya açılamadı:", name)
			os.Exit(1)
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		cerr := f.Close()
		if err != nil || cerr != nil {
			fmt.Fprintln(os.Stderr, "özet için dosya okunamadı:", name)
			os.Exit(1)
		}
		fmt.Printf("%x  %s\n", h.Sum(nil), name)
	}
}
