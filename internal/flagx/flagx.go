// Package flagx, bayrakların konumsal argümanlarla karışık verilebildiği
// (relay send --type soru "metin" --to x) komut satırı ayrıştırmasını sağlar.
package flagx

import "flag"

// Parse bayrakları ve konumsal argümanları ayırır. "--" sonrası her şey konumsaldır.
func Parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return pos, nil
		}
		if n := len(args) - len(rest); n > 0 && args[n-1] == "--" {
			return append(pos, rest...), nil
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
}
