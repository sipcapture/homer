package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func newTestUsersHandler(t *testing.T) (*UsersHandler, *AuthHandler, int64) {
	t.Helper()
	auth, userSvc := newTestAuthHandlerWithUsers(t)
	ctx := context.Background()

	id, err := userSvc.CreateUser(ctx, "alice", "secret", "alice@example.com", "Alice", false, true)
	if err != nil {
		t.Fatal(err)
	}

	return NewUsersHandler(userSvc), auth, id
}

func TestV4UsersUpdate_NonAdminCannotSelfAssignAdmin(t *testing.T) {
	h, auth, userID := newTestUsersHandler(t)
	e := echo.New()

	body := `{"user_group":"admin"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v4/users/"+strconv.FormatInt(userID, 10), strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("userId")
	c.SetParamValues(strconv.FormatInt(userID, 10))
	setJWTOnContext(t, c, auth, "alice", false)

	if err := h.V4UsersUpdate(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d want 403 body=%s", rec.Code, rec.Body.String())
	}

	user, err := h.userService.GetUserByID(context.Background(), userID)
	if err != nil || user == nil {
		t.Fatalf("GetUserByID: err=%v user=%v", err, user)
	}
	if user.IsAdmin {
		t.Fatal("user became admin after forbidden self-role escalation attempt")
	}
}

func TestV4UsersUpdate_NonAdminCannotChangeEnabled(t *testing.T) {
	h, auth, userID := newTestUsersHandler(t)
	e := echo.New()

	body := `{"enabled":false}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v4/users/"+strconv.FormatInt(userID, 10), strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("userId")
	c.SetParamValues(strconv.FormatInt(userID, 10))
	setJWTOnContext(t, c, auth, "alice", false)

	if err := h.V4UsersUpdate(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d want 403 body=%s", rec.Code, rec.Body.String())
	}
}

func TestV4UsersUpdate_NonAdminCanUpdateOwnEmail(t *testing.T) {
	h, auth, userID := newTestUsersHandler(t)
	e := echo.New()

	body := `{"email":"alice-new@example.com"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v4/users/"+strconv.FormatInt(userID, 10), strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("userId")
	c.SetParamValues(strconv.FormatInt(userID, 10))
	setJWTOnContext(t, c, auth, "alice", false)

	if err := h.V4UsersUpdate(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 body=%s", rec.Code, rec.Body.String())
	}

	user, err := h.userService.GetUserByID(context.Background(), userID)
	if err != nil || user == nil {
		t.Fatalf("GetUserByID: err=%v user=%v", err, user)
	}
	if user.Email != "alice-new@example.com" {
		t.Fatalf("email: got %q want alice-new@example.com", user.Email)
	}
	if user.IsAdmin {
		t.Fatal("unexpected admin elevation")
	}
}

func TestV4UsersUpdate_AdminCanChangeUserGroup(t *testing.T) {
	h, auth, userID := newTestUsersHandler(t)
	e := echo.New()

	body := `{"user_group":"admin"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v4/users/"+strconv.FormatInt(userID, 10), strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("userId")
	c.SetParamValues(strconv.FormatInt(userID, 10))
	setJWTOnContext(t, c, auth, "admin", true)

	if err := h.V4UsersUpdate(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 body=%s", rec.Code, rec.Body.String())
	}

	user, err := h.userService.GetUserByID(context.Background(), userID)
	if err != nil || user == nil {
		t.Fatalf("GetUserByID: err=%v user=%v", err, user)
	}
	if !user.IsAdmin {
		t.Fatal("admin update did not grant admin role")
	}
}
