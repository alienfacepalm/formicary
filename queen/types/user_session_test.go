package types

import (
	"testing"

	"github.com/stretchr/testify/require"
	common "plexobject.com/formicary/internal/types"
)

func Test_ShouldFailUserSessionValidationWithoutSessionID(t *testing.T) {
	session := NewUserSession(common.NewUser("org", "user", "", "email@formicary.io", false), "")
	require.Error(t, session.Validate())
	require.Contains(t, session.Validate().Error(), "session-id")
}

func Test_ShouldFailUserSessionValidationWithoutUsername(t *testing.T) {
	session := NewUserSession(common.NewUser("org", "", "", "email@formicary.io", false), "session")
	require.Error(t, session.Validate())
	require.Contains(t, session.Validate().Error(), "username")
}

func Test_ShouldFailUserSessionValidationWithoutUserID(t *testing.T) {
	session := NewUserSession(common.NewUser("org", "username", "", "email@formicary.io", false), "session")
	require.Error(t, session.Validate())
	require.Contains(t, session.Validate().Error(), "user-id")
}

func Test_ShouldVerifyUserSessionValidation(t *testing.T) {
	user := common.NewUser("org", "username", "", "email@formicary.io", false)
	user.ID = "blah"
	session := NewUserSession(user, "session")
	require.NoError(t, session.Validate())
	require.Equal(t, "formicary_user_sessions", session.TableName())
}
