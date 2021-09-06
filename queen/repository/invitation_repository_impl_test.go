package repository

import (
	"github.com/stretchr/testify/require"
	common "plexobject.com/formicary/internal/types"
	"plexobject.com/formicary/queen/types"
	"testing"
	"time"
)

// Get operation should fail if org doesn't exist
func Test_ShouldGetInvitationWithNonExistingId(t *testing.T) {
	// GIVEN repositories
	repo, err := NewTestInvitationRepository()
	require.NoError(t, err)
	// WHEN finding non-existing organization
	_, err = repo.Get("missing_id")

	// THEN it should fail
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

// Saving invitation
func Test_ShouldAddInvitation(t *testing.T) {
	// GIVEN repositories
	orgRepo, err := NewTestOrganizationRepository()
	require.NoError(t, err)
	invRepo, err := NewTestInvitationRepository()
	require.NoError(t, err)

	userRepo, err := NewTestUserRepository()
	require.NoError(t, err)

	orgRepo.Clear()
	userRepo.Clear()

	// AND an existing organization
	org, err := orgRepo.Create(qc, common.NewOrganization("user", "org1", "bundle1"))
	require.NoError(t, err)
	user := common.NewUser(org.ID, "user1", "name", "test@formicary.io", false)
	user, err = userRepo.Create(user)
	require.NoError(t, err)

	// WHEN adding an invitation
	inv := types.NewUserInvitation("touser@formicary.io", user, org)
	err = invRepo.Create(inv)
	require.NoError(t, err)
	loaded, err := invRepo.Get(inv.ID)
	require.NoError(t, err)

	// THEN Should find invitation
	loaded, err = invRepo.Find(inv.Email, inv.InvitationCode)
	require.NoError(t, err)
	require.Equal(t, inv.InvitedByUserID, loaded.InvitedByUserID)
	require.True(t, loaded.ExpiresAt.Unix() > time.Now().Unix())

	// AND should accept invitation
	_, err = invRepo.Accept(inv.Email, inv.InvitationCode)
	require.NoError(t, err)

	// BUT cannot accept again
	_, err = invRepo.Accept(inv.Email, inv.InvitationCode)
	require.Error(t, err)
}
