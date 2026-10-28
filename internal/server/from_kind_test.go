package server

import "testing"

// NEDEN (#31): ajan, görevi bir insan hesabının atadığını mesaj JSON'undan
// doğrulamalı; ayrıca `relay agents` çağırması gerekmemeli. from_kind sunucunun
// agents kaydından gelmeli (gövdeden/istemciden değil) ve boş olsa bile alan
// yanıtta bulunmalı (omitempty yok).
func TestMesajJSONGonderenTurunuTasir(t *testing.T) {
	e := newEnv(t, nil)
	// İstemci from_kind gönderse de sunucu yok saymalı: tür token sahibinin hesabından gelir.
	st, ajan := e.send("r1", "ali", map[string]any{"body": "ajan yazdı", "from_kind": "insan"})
	if st != 201 || ajan["from_kind"] != "ajan" {
		t.Fatalf("ajan gönderimi: %d %v", st, ajan)
	}
	st, insan := e.send("r1", "mehmet", map[string]any{"body": "insan yazdı"})
	if st != 201 || insan["from_kind"] != "insan" {
		t.Fatalf("insan gönderimi: %d %v", st, insan)
	}
	want := map[int]string{num(ajan["id"]): "ajan", num(insan["id"]): "insan"}
	msgs := e.fetch("r1", "veli", 0, 0)
	if len(msgs) != 2 {
		t.Fatalf("2 mesaj bekleniyordu: %v", msgs)
	}
	for _, m := range msgs {
		kind, ok := m["from_kind"]
		if !ok || kind != want[num(m["id"])] {
			t.Fatalf("from_kind yanlış/eksik: %v", m)
		}
	}
}
