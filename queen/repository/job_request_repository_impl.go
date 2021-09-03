package repository

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"runtime/debug"
	"sort"
	"strconv"
	"time"

	common "plexobject.com/formicary/internal/types"

	"github.com/twinj/uuid"
	"gorm.io/gorm"
	"plexobject.com/formicary/queen/types"
)

// JobRequestRepositoryImpl implements JobRequestRepository using gorm O/R mapping
type JobRequestRepositoryImpl struct {
	db     *gorm.DB
	dbType string
}

// NewJobRequestRepositoryImpl creates new instance for job-request-repository
func NewJobRequestRepositoryImpl(db *gorm.DB, dbType string) (*JobRequestRepositoryImpl, error) {
	return &JobRequestRepositoryImpl{db: db, dbType: dbType}, nil
}

// GetParams by id
func (jrr *JobRequestRepositoryImpl) GetParams(
	id uint64) (params []*types.JobRequestParam, err error) {
	if id == 0 {
		return nil, common.NewValidationError(
			fmt.Errorf("id is not specified for job-request"))
	}
	params = make([]*types.JobRequestParam, 0)
	tx := jrr.db.Where("job_request_id = ?", id).Limit(1000).Order("name")
	res := tx.Find(&params)
	if res.Error != nil {
		err = res.Error
		return nil, err
	}
	return
}

// Get method finds JobRequest by id
func (jrr *JobRequestRepositoryImpl) Get(
	qc *common.QueryContext,
	id uint64) (*types.JobRequest, error) {
	if id == 0 {
		return nil, common.NewValidationError(
			fmt.Errorf("id is not specified for job-request"))
	}
	var req types.JobRequest
	res := qc.AddOrgElseUserWhere(jrr.db).Preload("Params").Where("id = ?", id).First(&req)
	if res.Error != nil {
		return nil, common.NewNotFoundError(res.Error)
	}
	err := req.AfterLoad()
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Component": "JobRequestRepositoryImpl",
			"Error":     err,
		}).Warn("failed to initialize request after loading")
	}
	sort.Slice(req.Params, func(i, j int) bool { return req.Params[i].Name < req.Params[j].Name })
	return &req, nil
}

// GetByUserKey JobRequest by user-key
func (jrr *JobRequestRepositoryImpl) GetByUserKey(
	qc *common.QueryContext,
	userKey string) (*types.JobRequest, error) {
	var req types.JobRequest
	res := qc.AddOrgElseUserWhere(jrr.db).Preload("Params").Where("user_key = ?", userKey).First(&req)
	if res.Error != nil {
		return nil, common.NewNotFoundError(res.Error)
	}
	err := req.AfterLoad()
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Component": "JobRequestRepositoryImpl",
			"Error":     err,
		}).Warn("failed to initialize request after loading")
	}
	sort.Slice(req.Params, func(i, j int) bool { return req.Params[i].Name < req.Params[j].Name })
	return &req, nil
}

// Clear - for testing
func (jrr *JobRequestRepositoryImpl) Clear() {
	clearDB(jrr.db)
}

// UpdateJobState sets state of job-request
func (jrr *JobRequestRepositoryImpl) UpdateJobState(id uint64,
	oldState common.RequestState,
	newState common.RequestState,
	errorMessage string,
	errorCode string) error {
	var job types.JobRequest
	tx := jrr.db.Model(&job).Where("id = ?", id)
	if oldState != "" {
		tx = tx.Where("job_state = ?", oldState)
	}
	updates := map[string]interface{}{"job_state": newState, "updated_at": time.Now()}
	if errorMessage != "" || newState == common.COMPLETED {
		updates["error_message"] = errorMessage
		updates["error_code"] = errorCode
	}
	res := tx.Updates(updates)
	if res.Error != nil {
		return common.NewNotFoundError(res.Error)
	}
	if res.RowsAffected != 1 {
		return common.NewNotFoundError(
			fmt.Errorf("failed to update request state to %v with id %v", newState, id))
	}
	return nil
}

// UpdateRunningTimestamp sets updated time of job-request
func (jrr *JobRequestRepositoryImpl) UpdateRunningTimestamp(
	id uint64) error {
	var job types.JobRequest
	tx := jrr.db.Model(&job).
		Where("id = ?", id).
		Where("job_state IN (?)", []common.RequestState{common.EXECUTING, common.STARTED})
	res := tx.Updates(map[string]interface{}{"updated_at": time.Now()})
	if res.Error != nil {
		return common.NewNotFoundError(res.Error)
	}
	if res.RowsAffected != 1 {
		return common.NewNotFoundError(
			fmt.Errorf("failed to update request timestamp for %v", id))
	}
	return nil
}

// Save persists job-request
func (jrr *JobRequestRepositoryImpl) Save(
	req *types.JobRequest) (*types.JobRequest, error) {
	err := req.ValidateBeforeSave()
	if err != nil {
		return nil, common.NewValidationError(err)
	}
	err = jrr.db.Transaction(func(tx *gorm.DB) error {
		newReq := false
		if req.ID == 0 {
			req.CreatedAt = time.Now()
			req.UpdatedAt = time.Now()
			req.JobState = common.PENDING
			if req.UserKey == "" {
				req.UserKey = uuid.NewV4().String()
			}
			newReq = true
		} else {
			req.UpdatedAt = time.Now()
			jrr.clearOrphanJobParams(tx, req)
		}
		if req.UserKey == "" {
			req.UserKey = uuid.NewV4().String()
		}
		var res *gorm.DB

		for _, c := range req.Params {
			if c.ID == "" {
				c.ID = uuid.NewV4().String()
			}
			c.JobRequestID = req.ID
		}

		if newReq {
			res = tx.Create(req)
		} else {
			res = tx.Save(req)
		}

		if res.Error != nil {
			debug.PrintStack()
			return res.Error
		}

		//tx.Model(job).Association("Params").Replace(job.Params)
		return nil
	})
	return req, err
}

// SetReadyToExecute marks job as ready to execute so that job can be picked up by job launcher
func (jrr *JobRequestRepositoryImpl) SetReadyToExecute(
	id uint64,
	jobExecutionID string,
	lastJobExecutionID string) error {
	var totalExecutionCount int64
	res := jrr.db.Model(&types.JobExecution{}).
		Where("id = ?", jobExecutionID).Count(&totalExecutionCount)
	if res.Error != nil {
		return common.NewNotFoundError(res.Error)
	}
	if totalExecutionCount == 0 {
		return common.NewNotFoundError(
			fmt.Errorf("failed to find job-execution for %s", jobExecutionID))
	}

	var job types.JobRequest
	tx := jrr.db.Model(&job).
		Where("id = ?", id).
		Where("job_state = ?", common.PENDING)

	res = tx.Updates(map[string]interface{}{
		"job_state":             common.READY,
		"job_execution_id":      jobExecutionID,
		"last_job_execution_id": lastJobExecutionID,
		"updated_at":            time.Now(),
	})
	if res.Error != nil {
		return common.NewNotFoundError(res.Error)
	}
	if res.RowsAffected != 1 {
		old, err := jrr.Get(common.NewQueryContext("", "", ""), id)
		if err != nil {
			return common.NewNotFoundError(err)
		}
		return common.NewNotFoundError(
			fmt.Errorf("failed to mark job as ready because old status was %v for request-id %d",
				old.JobState, id))
	}
	return nil
}

// UpdatePriority update priority
func (jrr *JobRequestRepositoryImpl) UpdatePriority(
	qc *common.QueryContext,
	id uint64,
	priority int32) error {
	var job types.JobRequest
	tx := qc.AddOrgElseUserWhere(jrr.db.Model(&job)).Where("id = ?", id)
	res := tx.Updates(map[string]interface{}{"job_priority": priority, "updated_at": time.Now()})
	if res.Error != nil {
		return common.NewNotFoundError(res.Error)
	}
	if res.RowsAffected != 1 {
		return common.NewNotFoundError(
			fmt.Errorf("failed to update priority for job-request with id %v", id))
	}
	return nil
}

// Cancel a job
func (jrr *JobRequestRepositoryImpl) Cancel(
	qc *common.QueryContext,
	id uint64) error {
	req, err := jrr.Get(qc, id)
	if err != nil {
		return nil
	}
	if req.JobState.IsTerminal() {
		return common.NewConflictError(fmt.Sprintf("request %d is already in terminal state %s",
			req.ID, req.JobState))
	}
	return jrr.db.Transaction(func(db *gorm.DB) error {
		tx := qc.AddOrgElseUserWhere(db).Model(&types.JobRequest{}).Where("id = ?", id)
		res := tx.Updates(map[string]interface{}{
			"job_state":     common.CANCELLED,
			"error_code":    common.ErrorJobCancelled,
			"error_message": "cancelled by job request",
			"updated_at":    time.Now()})
		if res.Error != nil {
			return common.NewNotFoundError(res.Error)
		}
		if res.RowsAffected != 1 {
			return common.NewNotFoundError(
				fmt.Errorf("failed to mark job-request as cancel with id %v", id))
		}
		if req.JobExecutionID != "" {
			tx := qc.AddOrgElseUserWhere(db).Model(&types.JobExecution{}).Where("id = ?", req.JobExecutionID)
			res := tx.Updates(map[string]interface{}{
				"job_state":     common.CANCELLED,
				"error_message": "job request cancelled",
				"error_code":    common.ErrorJobCancelled,
				"updated_at":    time.Now(),
				"cpu_secs":      time.Now().Unix() - req.ScheduledAt.Unix(), // TODO verify
				"ended_at":      time.Now(),
			})

			if res.Error != nil {
				return common.NewNotFoundError(res.Error)
			}
		}
		return nil
	})
}

// Delete - deletes job-request
func (jrr *JobRequestRepositoryImpl) Delete(
	qc *common.QueryContext,
	id uint64) error {
	_, err := jrr.Get(qc, id)
	if err != nil {
		return err
	}
	return jrr.db.Transaction(func(db *gorm.DB) error {
		_ = db.Exec("DELETE FROM formicary_job_request_params WHERE job_request_id = ?", id)
		res := db.Exec("DELETE FROM formicary_job_requests WHERE id = ?", id)
		return res.Error
	})
}

// DeletePendingCronByJobType - delete pending cron job
func (jrr *JobRequestRepositoryImpl) DeletePendingCronByJobType(
	qc *common.QueryContext,
	jobType string) error {
	sql := "SELECT id FROM formicary_job_requests WHERE job_type = ? AND job_state = ? AND cron_triggered = ?"
	args := []interface{}{jobType, common.PENDING, true}
	if !qc.Admin() {
		if qc.OrganizationID != "" {
			sql += " AND organization_id = ?"
			args = append(args, qc.OrganizationID)
		} else if qc.UserID != "" {
			sql += " AND user_id = ?"
			args = append(args, qc.UserID)
		}
	}
	rows, err := jrr.db.Raw(sql, args...).Limit(100).Rows()
	if err != nil {
		return err
	}
	defer func() {
		_ = rows.Close()
	}()
	ids := make([]uint64, 0)

	for rows.Next() {
		var id jobRequestID
		if err = jrr.db.ScanRows(rows, &id); err != nil {
			return err
		}
		ids = append(ids, id.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	return jrr.db.Transaction(func(db *gorm.DB) error {
		res := jrr.db.Exec("DELETE FROM formicary_job_request_params WHERE job_request_id IN (?)", ids)
		if res.Error != nil {
			return common.NewNotFoundError(res.Error)
		}
		res = jrr.db.Exec("DELETE FROM formicary_job_requests WHERE id IN (?) AND job_state = ?", ids, common.PENDING)
		if res.Error != nil {
			return common.NewNotFoundError(res.Error)
		}
		return nil
	})
}

// Trigger triggers a scheduled job
func (jrr *JobRequestRepositoryImpl) Trigger(
	qc *common.QueryContext,
	id uint64) error {
	// TODO check for cron schedule
	sql := "UPDATE formicary_job_requests SET scheduled_at = ?, updated_at = ?, user_key = ? WHERE id = ? AND cron_triggered = ? AND job_state = ?"
	args := []interface{}{time.Now(), time.Now(), uuid.NewV4().String(), id, true, common.PENDING}
	if !qc.Admin() {
		if qc.OrganizationID != "" {
			sql += " AND organization_id = ?"
			args = append(args, qc.OrganizationID)
		} else if qc.UserID != "" {
			sql += " AND user_id = ?"
			args = append(args, qc.UserID)
		}
	}
	res := jrr.db.Exec(sql, args...)
	if res.Error != nil {
		return common.NewNotFoundError(res.Error)
	}
	return nil
}

// Restart the job
func (jrr *JobRequestRepositoryImpl) Restart(
	qc *common.QueryContext,
	id uint64) error {
	// TODO check for cron schedule
	sql := "UPDATE formicary_job_requests SET job_state = ?, last_job_execution_id = job_execution_id, " +
		"job_execution_id = NULL, error_code = NULL, error_message = NULL, schedule_attempts = 0, " +
		"retried = retried + 1, scheduled_at = ?, updated_at = ? WHERE id = ? AND job_state NOT IN (?)"
	args := []interface{}{common.PENDING, time.Now(), time.Now(), id, []common.RequestState{common.COMPLETED, common.PENDING}}
	if !qc.Admin() {
		if qc.OrganizationID != "" {
			sql += " AND organization_id = ?"
			args = append(args, qc.OrganizationID)
		} else if qc.UserID != "" {
			sql += " AND user_id = ?"
			args = append(args, qc.UserID)
		}
	}
	res := jrr.db.Exec(sql, args...)
	if res.Error != nil {
		return common.NewNotFoundError(res.Error)
	}
	if res.RowsAffected != 1 {
		return common.NewNotFoundError(
			fmt.Errorf("failed to restart job-request with id %v", id))
	}
	return nil
}

// IncrementScheduleAttempts and optionally bump schedule time and decrement priority for jobs that are not ready
func (jrr *JobRequestRepositoryImpl) IncrementScheduleAttempts(
	id uint64,
	scheduleSecs time.Duration,
	decrPriority int,
	errorMessage string) error {
	res := jrr.db.Exec(
		"UPDATE formicary_job_requests SET schedule_attempts = schedule_attempts + 1, "+
			"scheduled_at = ?, job_priority = job_priority - ?, error_message = ?, updated_at = ? WHERE id = ?",
		time.Now().Add(scheduleSecs*time.Second), decrPriority, errorMessage, time.Now(), id)
	if res.Error != nil {
		return common.NewNotFoundError(res.Error)
	}
	if res.RowsAffected != 1 {
		return common.NewNotFoundError(
			fmt.Errorf("failed to update schedule_attempts for %d", id))
	}
	return nil
}

type jobRequestID struct {
	ID uint64
}

// RecentDeadIDs returns recently completed job-ids
func (jrr *JobRequestRepositoryImpl) RecentDeadIDs(
	limit int) ([]uint64, error) {
	sql := "SELECT id FROM formicary_job_requests WHERE job_state IN (?) ORDER BY updated_at DESC limit ?"
	jobStates := []common.RequestState{common.FAILED, common.COMPLETED, common.CANCELLED}
	args := []interface{}{jobStates, limit}
	rows, err := jrr.db.Raw(sql, args...).Limit(limit).Rows()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	ids := make([]uint64, 0)

	for rows.Next() {
		var id jobRequestID
		if err = jrr.db.ScanRows(rows, &id); err != nil {
			return nil, err
		}
		ids = append(ids, id.ID)
	}

	return ids, nil

}

// JobCountsByDays calculates stats for all job-types/statuses within days
func (jrr *JobRequestRepositoryImpl) JobCountsByDays(
	qc *common.QueryContext,
	limit int,
) (stats []*types.JobCounts, err error) {
	var args []interface{}
	var sql string
	scopeSelect := ""
	scopeWhere := ""
	if qc.Admin() {
	} else if qc.OrganizationID != "" {
		scopeSelect = "organization_id,"
		scopeWhere = "WHERE organization_id = ? "
		args = append(args, qc.OrganizationID)
	} else if qc.UserID != "" {
		scopeSelect = "user_id,"
		scopeWhere = "WHERE user_id = ? "
		args = append(args, qc.UserID)
	}

	if jrr.dbType == "sqlite" {
		sql = "SELECT " + scopeSelect + "job_type, job_state, count(*) AS counts, date(updated_at) as day " +
			" FROM formicary_job_requests " + scopeWhere +
			" GROUP BY " + scopeSelect + "job_type, job_state, day ORDER BY day desc limit ?"
	} else {
		sql = "SELECT " + scopeSelect + "job_type, job_state, count(*) AS counts, cast(updated_at as date) as start_time, " +
			" cast(updated_at as date) as end_time FROM formicary_job_requests " + scopeWhere +
			" GROUP BY " + scopeSelect + "job_type, job_state, start_time ORDER BY start_time desc limit ?"
	}
	args = append(args, limit)
	rows, err := jrr.db.Raw(sql, args...).Rows()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	stats = make([]*types.JobCounts, 0)

	for rows.Next() {
		stat := types.JobCounts{}
		if err = jrr.db.ScanRows(rows, &stat); err != nil {
			return nil, err
		}
		if stat.Day != "" {
			stat.StartTime, _ = time.Parse("2006-01-02", stat.Day)
			stat.EndTime, _ = time.Parse("2006-01-02", stat.Day)
		} else {
			stat.Day = stat.GetStartTime().Format("2006-01-02")
		}
		stats = append(stats, &stat)
	}
	logrus.WithFields(logrus.Fields{
		"Component": "JobRequestRepositoryImpl",
		"Method":    "JobCountsByDays",
		"SQL":       sql,
		"Args":      args,
		"Stats":     len(stats),
	}).Infof("job counts by days")
	return stats, nil
}

// JobCounts calculates stats for all job-types/statuses/error-codes within given range
func (jrr *JobRequestRepositoryImpl) JobCounts(
	qc *common.QueryContext,
	start time.Time,
	end time.Time) ([]*types.JobCounts, error) {
	args := []interface{}{start, end}
	scopeSelect := ""
	scopeWhere := ""
	if qc.Admin() {
	} else if qc.OrganizationID != "" {
		scopeSelect = "organization_id,"
		scopeWhere = "AND organization_id = ? "
		args = append(args, qc.OrganizationID)
	} else if qc.UserID != "" {
		scopeSelect = "user_id,"
		scopeWhere = "AND user_id = ? "
		args = append(args, qc.UserID)
	}
	var sql string
	if jrr.dbType == "sqlite" {
		sql = "SELECT " + scopeSelect + "job_type, job_state, error_code, count(*) AS counts, min(updated_at) as start_time_string, " +
			"max(updated_at) as end_time_string FROM formicary_job_requests WHERE updated_at >= ? AND updated_at <= ? " +
			scopeWhere + " GROUP BY " + scopeSelect + "job_type, job_state, error_code ORDER BY job_type "
	} else {
		sql = "SELECT " + scopeSelect + "job_type, job_state, error_code, count(*) AS counts, min(updated_at) as start_time, " +
			"max(updated_at) as end_time FROM formicary_job_requests WHERE updated_at >= ? AND updated_at <= ? " +
			scopeWhere + " GROUP BY " + scopeSelect + "job_type, job_state, error_code ORDER BY job_type "
	}

	rows, err := jrr.db.Raw(sql, args...).Rows()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	stats := make([]*types.JobCounts, 0)

	for rows.Next() {
		stat := types.JobCounts{}
		if err = jrr.db.ScanRows(rows, &stat); err != nil {
			return nil, err
		}
		if stat.StartTime.Unix() >= start.Unix() && stat.EndTime.Unix() <= end.Unix() {
			stat.Day = stat.GetStartTime().Format("2006-01-02")
			stats = append(stats, &stat)
		}
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].EndTime.Unix() > stats[j].EndTime.Unix() })
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		logrus.WithFields(logrus.Fields{
			"Component": "JobRequestRepositoryImpl",
			"Method":    "JobCounts",
			"SQL":       sql,
			"Args":      args,
			"Stats":     len(stats),
		}).Debugf("job counts")
	}
	return stats, nil
}

// CountByOrgAndState returns counts
func (jrr *JobRequestRepositoryImpl) CountByOrgAndState(
	org string,
	state common.RequestState) (totalRecords int64, err error) {
	res := jrr.db.Model(&types.JobRequest{}).
		Where("organization_id = ?", org).
		Where("job_state = ?", state).Count(&totalRecords)
	if res.Error != nil {
		err = res.Error
	}
	return
}

// FindActiveCronScheduledJobsByJobType queries scheduled jobs that are either running or waiting to be run
func (jrr *JobRequestRepositoryImpl) FindActiveCronScheduledJobsByJobType(
	jobTypesAndTrigger []types.JobTypeCronTrigger,
) ([]*types.JobRequestInfo, error) {
	// initialize job-types, user-ids, and keys
	jobTypes := make([]string, len(jobTypesAndTrigger))
	userIDs := make([]string, 0)
	userKeys := make([]string, len(jobTypesAndTrigger))

	for i, typeAndTrigger := range jobTypesAndTrigger {
		jobTypes[i] = typeAndTrigger.JobType
		if typeAndTrigger.UserID != "" {
			userIDs = append(userIDs, typeAndTrigger.UserID)
		}
		_, userKeys[i] = types.GetCronScheduleTimeAndUserKey(typeAndTrigger.OrganizationOrUserID(), typeAndTrigger.JobType, typeAndTrigger.CronTrigger)
	}
	jobStates := []common.RequestState{common.PENDING, common.READY, common.STARTED, common.EXECUTING}
	args := []interface{}{true, jobTypes, jobStates}

	sql := "SELECT id, job_type, job_version, organization_id, user_id, job_priority, job_state, schedule_attempts, scheduled_at, created_at, " +
		" job_definition_id, job_execution_id, last_job_execution_id, cron_triggered FROM formicary_job_requests WHERE " +
		" cron_triggered = ? AND ((job_type IN (?) AND job_state IN (?)"
	if len(userIDs) > 0 {
		sql += " AND user_id IN (?)"
		args = append(args, userIDs)
	}
	sql += ") OR user_key IN (?)) order by updated_at desc"
	args = append(args, userKeys)

	rows, err := jrr.db.Raw(sql, args...).Rows()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	infos := make([]*types.JobRequestInfo, 0)

	duplicates := make(map[string]bool)
	for rows.Next() {
		info := types.JobRequestInfo{}
		if err = jrr.db.ScanRows(rows, &info); err != nil {
			return nil, err
		}
		if !duplicates[info.GetUserJobTypeKey()] {
			infos = append(infos, &info)
			duplicates[info.GetUserJobTypeKey()] = true
		}
	}
	return infos, nil
}

// GetJobTimes returns job times for recent jobs
func (jrr *JobRequestRepositoryImpl) GetJobTimes(
	limit int) ([]*types.JobTime, error) {
	sql := "SELECT r.id, r.organization_id, r.user_id, r.job_type, r.job_version, r.job_state, " +
		" r.job_priority, r.scheduled_at, r.created_at , x.started_at, x.ended_at " +
		"FROM formicary_job_requests r LEFT OUTER JOIN formicary_job_executions x " +
		"ON r.job_execution_id = x.id ORDER BY r.updated_at DESC LIMIT ?"
	rows, err := jrr.db.Raw(sql, limit).Limit(limit).Rows()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	times := make([]*types.JobTime, 0)

	for rows.Next() {
		jt := types.JobTime{}
		if err = jrr.db.ScanRows(rows, &jt); err != nil {
			return nil, err
		}
		times = append(times, &jt)
	}
	return times, nil
}

// NextSchedulableJobsByType queries basic job id/state for pending/ready state from parameter
func (jrr *JobRequestRepositoryImpl) NextSchedulableJobsByType(
	jobTypes []string,
	state common.RequestState,
	limit int) ([]*types.JobRequestInfo, error) {
	sql := "SELECT id, job_type, job_version, organization_id, user_id, job_priority, job_state, schedule_attempts, scheduled_at, created_at, " +
		" job_definition_id, job_execution_id, last_job_execution_id, cron_triggered FROM formicary_job_requests WHERE job_type in " +
		" (SELECT job_type FROM formicary_job_definitions where paused is false and active is true)" +
		" AND job_state = ? AND scheduled_at <= ? ORDER BY job_priority DESC, created_at LIMIT ?"

	args := []interface{}{state, time.Now(), limit}

	if len(jobTypes) > 0 {
		sql += " AND job_type in (?)"
		args = append(args, jobTypes)
	}
	rows, err := jrr.db.Raw(sql, args...).Rows()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	infos := make([]*types.JobRequestInfo, 0)

	for rows.Next() {
		info := types.JobRequestInfo{}
		if err = jrr.db.ScanRows(rows, &info); err != nil {
			return nil, err
		}
		infos = append(infos, &info)
	}
	return infos, nil
}

// RequeueOrphanRequests queries jobs with EXECUTING/STARTED status and puts them back to PENDING
func (jrr *JobRequestRepositoryImpl) RequeueOrphanRequests(
	staleInterval time.Duration) (total int64, err error) {
	date := time.Now().Add(-staleInterval)
	res := jrr.db.Exec(
		"UPDATE formicary_job_requests SET job_state = ?, scheduled_at = ?, updated_at = ? "+
			" WHERE job_state IN (?) AND updated_at < ?",
		common.PENDING,
		time.Now(),
		time.Now(),
		[]common.RequestState{common.STARTED, common.READY, common.EXECUTING},
		date)
	if res.Error != nil {
		return 0, res.Error
	}
	if res.RowsAffected == 0 && logrus.IsLevelEnabled(logrus.DebugLevel) {
		logrus.WithFields(logrus.Fields{
			"Component": "JobRequestRepositoryImpl",
			"Date":      date,
		}).Debugf("didn't find any orphan jobs")
	}
	return res.RowsAffected, nil
}

// QueryOrphanRequests queries orphan jobs that haven't been updated
func (jrr *JobRequestRepositoryImpl) QueryOrphanRequests(
	limit int,
	offset int,
	staleInterval time.Duration) (jobRequests []*types.JobRequest, err error) {
	jobRequests = make([]*types.JobRequest, 0)
	date := time.Now().Add(-staleInterval * time.Second)
	tx := jrr.db.Preload("Params").Limit(limit).Offset(offset).
		Where("job_state IN (?)", []common.RequestState{common.STARTED, common.READY, common.EXECUTING}).
		Where("updated_at < ?", date).Order("updated_at")
	res := tx.Find(&jobRequests)
	if res.Error != nil {
		err = res.Error
		return nil, err
	}
	for _, j := range jobRequests {
		err = j.AfterLoad()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"Component": "JobRequestRepositoryImpl",
				"Error":     err,
			}).Warn("failed to initialize request after loading for orphan requests")
		}
	}
	return
}

// Query finds matching job-request by parameters
func (jrr *JobRequestRepositoryImpl) Query(
	qc *common.QueryContext,
	params map[string]interface{},
	page int,
	pageSize int,
	order []string) (jobRequests []*types.JobRequest, totalRecords int64, err error) {
	jobState := params["job_state"]
	jobRequests = make([]*types.JobRequest, 0)
	tx := qc.AddOrgElseUserWhere(jrr.db).Preload("Params").Limit(pageSize).Offset(page * pageSize)
	q := params["q"]
	if q != nil {
		reqID, _ := strconv.ParseInt(fmt.Sprintf("%s", q), 10, 64)
		qs := fmt.Sprintf("%%%s%%", q)
		tx = tx.Where("id = ? OR job_type LIKE ? OR description LIKE ? OR user_id LIKE ? OR organization_id LIKE ? OR quick_search LIKE ?",
			reqID, qs, qs, qs, qs, qs)
	} else {
		tx = jrr.addQueryParams(params, tx)
	}
	if len(order) == 0 {
		if jobState == common.WAITING || jobState == common.READY || jobState == common.PENDING {
			tx = tx.Order("job_priority DESC").Order("created_at")
		} else {
			tx = tx.Order("updated_at DESC")
		}
	} else {
		for _, ord := range order {
			tx = tx.Order(ord)
		}
	}
	res := tx.Find(&jobRequests)
	if res.Error != nil {
		err = res.Error
		return nil, 0, err
	}
	for _, j := range jobRequests {
		err = j.AfterLoad()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"Component": "JobRequestRepositoryImpl",
				"Error":     err,
			}).Warn("failed to initialize request after loading for query")
		}
	}
	if jobState != nil {
		params["job_state"] = jobState
	}
	totalRecords, _ = jrr.Count(qc, params)
	return
}

func (jrr *JobRequestRepositoryImpl) addQueryParams(
	params map[string]interface{}, tx *gorm.DB) *gorm.DB {
	jobState := params["job_state"]
	if jobState != nil {
		delete(params, "job_state")
		if jobState == "RUNNING" {
			tx = tx.Where("job_state IN (?)", []common.RequestState{common.STARTED, common.EXECUTING})
		} else if jobState == "WAITING" {
			tx = tx.Where("job_state IN (?)", []common.RequestState{common.PENDING, common.READY})
		} else if jobState == "DONE" {
			tx = tx.Where("job_state IN (?)",
				[]common.RequestState{common.FAILED, common.COMPLETED, common.CANCELLED})
		} else {
			tx = tx.Where("job_state = ?", jobState)
		}
	}
	return addQueryParamsWhere(params, tx)
}

// Count counts records by query
func (jrr *JobRequestRepositoryImpl) Count(
	qc *common.QueryContext,
	params map[string]interface{}) (totalRecords int64, err error) {
	tx := jrr.db.Model(&types.JobRequest{})
	tx = qc.AddOrgElseUserWhere(tx)
	tx = jrr.addQueryParams(params, tx)
	res := tx.Count(&totalRecords)
	if res.Error != nil {
		err = res.Error
	}
	return
}

// clearOrphanJobParams remove orphan request params
func (jrr *JobRequestRepositoryImpl) clearOrphanJobParams(tx *gorm.DB, req *types.JobRequest) {
	paramIDs := make([]string, len(req.Params))
	for i, c := range req.Params {
		paramIDs[i] = c.ID
	}

	tx.Where("id NOT IN (?) AND job_request_id = ?", paramIDs, req.ID).Delete(types.JobRequestParam{})
}
