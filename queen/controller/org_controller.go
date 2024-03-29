package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"plexobject.com/formicary/internal/utils"
	"time"

	"plexobject.com/formicary/internal/acl"
	common "plexobject.com/formicary/internal/types"
	"plexobject.com/formicary/internal/web"
	"plexobject.com/formicary/queen/manager"
	"plexobject.com/formicary/queen/types"
)

// OrganizationController structure
type OrganizationController struct {
	userManager *manager.UserManager
	webserver   web.Server
}

// NewOrganizationController instantiates controller for updating orgs
func NewOrganizationController(
	userManager *manager.UserManager,
	webserver web.Server) *OrganizationController {
	orgCtrl := &OrganizationController{
		userManager: userManager,
		webserver:   webserver,
	}
	webserver.GET("/api/orgs", orgCtrl.queryOrganizations, acl.NewPermission(acl.Organization, acl.Query)).Name = "query_orgs"
	webserver.GET("/api/orgs/:id", orgCtrl.getOrganization, acl.NewPermission(acl.Organization, acl.View)).Name = "get_org"
	webserver.POST("/api/orgs", orgCtrl.postOrganization, acl.NewPermission(acl.Organization, acl.Create)).Name = "create_org"
	webserver.PUT("/api/orgs/:id", orgCtrl.putOrganization, acl.NewPermission(acl.Organization, acl.Update)).Name = "update_org"
	webserver.DELETE("/api/orgs/:id", orgCtrl.deleteOrganization, acl.NewPermission(acl.Organization, acl.Delete)).Name = "delete_org"
	webserver.POST("/api/orgs/:id/invite", orgCtrl.inviteUser, acl.NewPermission(acl.UserInvitation, acl.Update)).Name = "accept_invitation"
	webserver.GET("/api/orgs/usage_report", orgCtrl.usageReport, acl.NewPermission(acl.Report, acl.View)).Name = "admin_usage_report"
	return orgCtrl
}

// ********************************* HTTP Handlers ***********************************

// swagger:route GET /api/orgs organizations queryOrganizations
// Queries organizations by criteria such as org-unit, bundle, etc.
// `This requires admin access`
// responses:
//
//	200: orgQueryResponse
func (oc *OrganizationController) queryOrganizations(c web.APIContext) error {
	qc := web.BuildQueryContext(c)
	params, order, page, pageSize, _, _ := ParseParams(c)
	recs, total, err := oc.userManager.QueryOrgs(qc, params, page, pageSize, order)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, NewPaginatedResult(recs, total, page, pageSize))
}

// swagger:route POST /api/orgs organizations postOrganization
// Creates new organization.
// `This requires admin access`
// responses:
//
//	200: orgResponse
func (oc *OrganizationController) postOrganization(c web.APIContext) error {
	qc := web.BuildQueryContext(c)
	now := time.Now()
	org := common.NewOrganization("", "", "")
	err := json.NewDecoder(c.Request().Body).Decode(org)
	if err != nil {
		return err
	}
	saved, err := oc.userManager.CreateOrg(qc, org)
	if err != nil {
		return err
	}
	status := 0
	if saved.CreatedAt.Unix() >= now.Unix() {
		status = http.StatusCreated
	} else {
		status = http.StatusOK
	}
	return c.JSON(status, saved)
}

// swagger:route PUT /api/orgs/{id} organizations putOrganization
// Updates the organization profile.
// responses:
//
//	200: orgResponse
func (oc *OrganizationController) putOrganization(c web.APIContext) error {
	org := common.NewOrganization("", "", "")
	err := json.NewDecoder(c.Request().Body).Decode(org)
	if err != nil {
		return err
	}
	qc := web.BuildQueryContext(c)
	org.ID = qc.GetOrganizationID()
	saved, err := oc.userManager.UpdateOrg(qc, org)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, saved)
}

// swagger:route GET /api/orgs/{id} organizations getOrganization
// Finds the organization by its id.
// responses:
//
//	200: orgResponse
func (oc *OrganizationController) getOrganization(c web.APIContext) error {
	qc := web.BuildQueryContext(c)
	org, err := oc.userManager.GetOrganization(qc, c.Param("id"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, org)
}

// swagger:route DELETE /api/orgs/{id} organizations deleteOrganization
// Deletes the organization by its id.
// responses:
//
//	200: emptyResponse
func (oc *OrganizationController) deleteOrganization(c web.APIContext) error {
	qc := web.BuildQueryContext(c)
	err := oc.userManager.DeleteOrganization(qc, c.Param("id"))
	if err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// swagger:route POST /api/orgs/{id}/invite organizations inviteUser
// Invite a user to the organization.
// responses:
//
//	200: userInvitationResponse
func (oc *OrganizationController) inviteUser(c web.APIContext) (err error) {
	qc := web.BuildQueryContext(c)
	user := web.GetDBLoggedUserFromSession(c)
	if user == nil {
		return fmt.Errorf("failed to find user in session for invitation")
	}
	inv := &types.UserInvitation{}
	err = json.NewDecoder(c.Request().Body).Decode(inv)
	if err != nil {
		return err
	}
	if err = oc.userManager.InviteUser(qc, user, inv); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, inv)
}

// swagger:route POST /api/orgs/usage_report organizations usageReport
// Generates usage report for the organization.
// `This requires admin access`
// Shows usage report by organization and user
// responses:
//
//	200: usageReportResponse
func (oc *OrganizationController) usageReport(c web.APIContext) error {
	from := utils.ParseStartDateTime(c.QueryParam("from"))
	to := utils.ParseEndDateTime(c.QueryParam("to"))
	combinedUsage := oc.userManager.CombinedResourcesByOrgUser(from, to, 10000)
	return c.JSON(http.StatusOK, combinedUsage)
}

// ********************************* Swagger types ***********************************

// swagger:parameters queryOrgs
// The params for querying orgs.
type orgQueryParams struct {
	// in:query
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	// BundleID defines package or bundle
	BundleID string `json:"bundle_id"`
	// OrgUnit defines org-unit
	OrgUnit string `json:"org_unit"`
}

// Paginated results of orgs matching query
// swagger:response orgQueryResponse
type orgQueryResponseBody struct {
	// in:body
	Body struct {
		Records      []common.Organization
		TotalRecords int64
		Page         int
		PageSize     int
		TotalPages   int64
	}
}

// swagger:parameters orgIDParams deleteOrganization getOrganization putOrganization inviteUser
// The parameters for finding organization by id
type orgIDParams struct {
	// in:path
	ID string `json:"id"`
}

// swagger:parameters usageReport
// The parameters for finding usage report
type usageReport struct {
	// in:query
	// From ISO date
	From string `json:"from"`
	// TO ISO date
	To string `json:"to"`
}

// swagger:parameters postOrganization
// The request body includes organization for persistence.
type orgCreateParams struct {
	// in:body
	Body common.Organization
}

// swagger:parameters putOrganization
// The request body includes organization for persistence.
type orgUpdateParams struct {
	// in:path
	ID string `json:"id"`
	// in:body
	Body common.Organization
}

// Org defines user request to process a job, which is saved in the database as PENDING and is then scheduled for job execution.
// swagger:response orgResponse
type orgResponseBody struct {
	// in:body
	Body common.Organization
}

// User invitation
// swagger:response userInvitationResponse
type userInvitationResponseBody struct {
	// in:body
	Body types.UserInvitation
}

// Usage Report
// swagger:response usageReportResponse
type usageReportResponse struct {
	// in:body
	Body []types.CombinedResourceUsage
}
