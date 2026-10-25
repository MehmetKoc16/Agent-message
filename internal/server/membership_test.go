package server

import (
	"context"
	"github.com/MehmetKoc16/Agent-message/internal/store"
	"testing"
	"time"
)

func TestAdminRoomMembershipDoesNotChangeAuthority(t *testing.T) {
	e, now := yonetimEnv(t)
	ctx := context.Background()
	if err := e.st.AddAgent(ctx, "other-admin", "insan", store.HashToken("test-other-admin"), nil); err != nil {
		t.Fatal(err)
	}
	if err := e.st.SetYonetici(ctx, "other-admin", testTOTPSecret); err != nil {
		t.Fatal(err)
	}
	session := yonetimLogin(t, e, *now)
	path := "/hesaplar/other-admin/odalar"
	yonetimWant(t, yonetimReq(e, "POST", path, "ali", "", `{"oda":"r1"}`), 403, "yalniz_yonetici")
	yonetimWant(t, yonetimReq(e, "POST", path, "mehmet", "", `{"oda":"r1"}`), 401, "yonetim_oturumu_gerekli")
	for i := 0; i < 2; i++ {
		yonetimWant(t, yonetimReq(e, "POST", path, "mehmet", session, `{"oda":"r1"}`), 200, "")
	}
	member, err := e.st.HasRoom(ctx, "other-admin", "r1")
	if err != nil || !member {
		t.Fatalf("member=%v err=%v", member, err)
	}
	yonetimWant(t, yonetimReq(e, "DELETE", path+"/r1", "mehmet", session, ""), 200, "")
	member, err = e.st.HasRoom(ctx, "other-admin", "r1")
	if err != nil || member {
		t.Fatalf("member=%v err=%v", member, err)
	}
	yes, err := e.st.IsYonetici(ctx, "other-admin")
	if err != nil || !yes {
		t.Fatal("membership changed admin role", err)
	}
	yonetimWant(t, yonetimReq(e, "POST", "/hesaplar/other-admin/iptal", "mehmet", session, ""), 403, "yoneticiye_dokunulamaz")
	*now = now.Add(10 * time.Minute)
	yonetimWant(t, yonetimReq(e, "POST", path, "mehmet", session, `{"oda":"r1"}`), 401, "yonetim_oturumu_gerekli")
}

func TestManagementRoomCatalogRequiresAdminAndTOTP(t *testing.T) {
	e, now := yonetimEnv(t)
	yonetimWant(t, yonetimReq(e, "GET", "/odalar", "ali", "", ""), 403, "yalniz_yonetici")
	yonetimWant(t, yonetimReq(e, "GET", "/odalar", "mehmet", "", ""), 401, "yonetim_oturumu_gerekli")
	session := yonetimLogin(t, e, *now)
	yonetimWant(t, yonetimReq(e, "GET", "/odalar", "mehmet", session, ""), 200, "")
}
