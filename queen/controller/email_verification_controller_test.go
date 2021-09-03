package controller

import (
	"bytes"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/url"
	common "plexobject.com/formicary/internal/types"
	"plexobject.com/formicary/internal/web"
	"plexobject.com/formicary/queen/repository"
	"plexobject.com/formicary/queen/types"
	"strings"
	"testing"
)

func Test_InitializeSwaggerStructsForEmailVerification(t *testing.T) {
	_ = emailVerificationQueryParams{}
	_ = emailVerificationQueryResponseBody{}

	_ = emailVerificationCreateParams{}
	_ = emailVerificationVerifyParams{}
	_ = emailVerificationCreateResponseBody{}
}

func Test_ShouldQueryEmailVerifications(t *testing.T) {
	// GIVEN error-code controller
	emailVerifyRepository, err := repository.NewTestEmailVerificationRepository()
	require.NoError(t, err)
	u := common.NewUser("", "username", "name", false)
	u.ID = "user-id"
	u.Email = "email@mail.com"
	userRepository, err := repository.NewTestUserRepository()
	u, err = userRepository.Create(u)
	require.NoError(t, err)

	ev := types.NewEmailVerification("email@mail.com", u)
	_, err = emailVerifyRepository.Create(ev)
	require.NoError(t, err)
	webServer := web.NewStubWebServer()
	ctrl := NewEmailVerificationController(newTestUserManager(newTestConfig(), t), webServer)

	// WHEN querying error codes
	reader := io.NopCloser(strings.NewReader(""))
	req := &http.Request{Body: reader, URL: &url.URL{}}
	ctx := web.NewStubContext(req)
	err = ctrl.queryEmailVerifications(ctx)

	// THEN it should not fail nad return error codes
	require.NoError(t, err)
	recs := ctx.Result.(*PaginatedResult).Records.([]*types.EmailVerification)
	require.NotEqual(t, 0, len(recs))
}

func Test_ShouldCreateAndGetEmailVerification(t *testing.T) {
	// GIVEN error-code controller
	webServer := web.NewStubWebServer()
	ctrl := NewEmailVerificationController(newTestUserManager(newTestConfig(), t), webServer)
	userRepository, err := repository.NewTestUserRepository()
	require.NoError(t, err)
	userRepository.Clear()

	// WHEN creating error-code
	u := common.NewUser("", "username", "name", false)
	u.ID = "user-id"
	u.Email = "email@mail.com"
	u, err = userRepository.Create(u)

	require.NoError(t, err)
	ev := types.NewEmailVerification("email@mail.com", u)
	b, err := json.Marshal(ev)
	require.NoError(t, err)
	reader := io.NopCloser(bytes.NewReader(b))
	ctx := web.NewStubContext(&http.Request{Body: reader})
	ctx.Set(web.DBUser, u)
	err = ctrl.createEmailVerification(ctx)

	// THEN it should not fail and add error code
	require.NoError(t, err)
	emailVerify := ctx.Result.(*types.EmailVerification)
	require.NotEqual(t, "", emailVerify.ID)

	// WHEN getting error code
	ctx.Params["code"] = emailVerify.EmailCode
	err = ctrl.queryEmailVerifications(ctx)
	// THEN it should not fail and add error code
	require.NoError(t, err)
	recs := ctx.Result.(*PaginatedResult).Records.([]*types.EmailVerification)
	require.Equal(t, 1, len(recs))
}

func Test_ShouldUpdateAndVerifyEmailVerification(t *testing.T) {
	// GIVEN error-code controller
	webServer := web.NewStubWebServer()
	ctrl := NewEmailVerificationController(newTestUserManager(newTestConfig(), t), webServer)
	userRepository, err := repository.NewTestUserRepository()
	require.NoError(t, err)
	userRepository.Clear()

	// WHEN updating error code
	u := common.NewUser("", "username", "name", false)
	u.ID = "user-id"
	u.Email = "email@mail.com"
	u, err = userRepository.Create(u)
	require.NoError(t, err)
	ev := types.NewEmailVerification("email@mail.com", u)
	b, err := json.Marshal(ev)
	require.NoError(t, err)
	reader := io.NopCloser(bytes.NewReader(b))
	ctx := web.NewStubContext(&http.Request{Body: reader})
	ctx.Set(web.DBUser, u)
	err = ctrl.createEmailVerification(ctx)

	// THEN it should not fail and update error code
	require.NoError(t, err)
	emailVerify := ctx.Result.(*types.EmailVerification)
	require.NotEqual(t, "", emailVerify.ID)

	// WHEN getting error code
	ctx.Params["id"] = emailVerify.UserID
	ctx.Params["code"] = emailVerify.EmailCode
	err = ctrl.verifyEmailVerification(ctx)
	// THEN it should not fail and update error code
	require.NoError(t, err)
	saved := ctx.Result.(*types.EmailVerification)
	require.Equal(t, emailVerify.EmailCode, saved.EmailCode)
}
