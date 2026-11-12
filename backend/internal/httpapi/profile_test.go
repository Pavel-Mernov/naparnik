package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProfileAPI(t *testing.T) {
	_, h := newServer(t)
	if code, _ := call(t, h, "GET", "/api/profile", "", ""); code != 401 {
		t.Fatal(code)
	}
	code, out := call(t, h, "GET", "/api/profile", "intern.sokolov", "")
	p, _ := out["profile"].(map[string]any)
	if code != 200 || p["status"] != "Стажёр" || p["email"] != "intern.sokolov@naparnik.example" || p["photo"] != nil || p["can_edit"] != true {
		t.Fatal(code, out)
	}
	code, out = call(t, h, "GET", "/api/profiles?role=intern", "lead.kovalev", "")
	if code != 200 || len(out["profiles"].([]any)) != 2 {
		t.Fatal(code, out)
	}
	if code, _ := call(t, h, "GET", "/api/profiles?role=admin", "lead.kovalev", ""); code != 422 {
		t.Fatal(code)
	}
	if code, _ := call(t, h, "GET", "/api/profiles/u-nobody", "lead.kovalev", ""); code != 404 {
		t.Fatal(code)
	}
	code, out = call(t, h, "PATCH", "/api/profile", "intern.sokolov", `{"phone":"+7 912 345-67-89"}`)
	if code != 200 || out["profile"].(map[string]any)["phone"] != "+7 912 345-67-89" {
		t.Fatal(code, out)
	}
	for _, body := range []string{`{"name":"Хакер"}`, `{"phone":"x"}`, `{"email":"a@b.c","phone":"+7 912 345-67-89"}`, `не json`} {
		if code, _ := call(t, h, "PATCH", "/api/profile", "intern.sokolov", body); code != 422 {
			t.Fatal(body, code)
		}
	}

	put := func(user, ct string, body []byte) int {
		req := httptest.NewRequest("PUT", "/api/profile/photo", strings.NewReader(string(body)))
		req.Header.Set("X-User", user)
		req.Header.Set("Content-Type", ct)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	if c := put("intern.sokolov", "image/png", []byte("<svg onload=alert(1)>")); c != 422 {
		t.Fatal("svg и html не принимаются", c)
	}
	if c := put("intern.sokolov", "image/png", png); c != 200 {
		t.Fatal(c)
	}
	req := httptest.NewRequest("GET", "/api/profiles/u-sokolov/photo", nil)
	req.Header.Set("X-User", "buddy.belova")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" || rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Body.Len() != len(png) {
		t.Fatal(rec.Code, rec.Header())
	}
	if code, _ := call(t, h, "DELETE", "/api/profile/photo", "intern.sokolov", ""); code != 200 {
		t.Fatal(code)
	}
	req = httptest.NewRequest("GET", "/api/profiles/u-sokolov/photo", nil)
	req.Header.Set("X-User", "buddy.belova")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatal(rec.Code)
	}
}
