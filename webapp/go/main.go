package main

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/redis/go-redis/v9"
	"github.com/samber/lo"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/sync/singleflight"
)

const (
	SQLDirectory         = "../sql/pg/"
	AssignmentsDirectory = "../assignments/"
	InitDataDirectory    = "../data/"
	SessionName          = "isucholar_go"
)

type handlers struct {
	DB *sqlx.DB
}

func main() {
	tp, _ := initTracer(context.Background())
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			panic(err)
		}
	}()
	initProfile()

	e := echo.New()
	e.Debug = GetEnv("DEBUG", "") == "true"
	e.Server.Addr = fmt.Sprintf(":%v", GetEnv("PORT", "7000"))
	e.HideBanner = true

	// e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(session.Middleware(sessions.NewCookieStore([]byte("trapnomura"))))
	e.Use(otelecho.Middleware("isucholar"))

	db, _ := GetDBOtel()
	db.SetMaxOpenConns(10)

	h := &handlers{
		DB: db,
	}

	e.POST("/initialize", h.Initialize)

	e.POST("/login", h.Login)
	e.POST("/logout", h.Logout)
	API := e.Group("/api", h.IsLoggedIn)
	{
		usersAPI := API.Group("/users")
		{
			usersAPI.GET("/me", h.GetMe)
			usersAPI.GET("/me/courses", h.GetRegisteredCourses)
			usersAPI.PUT("/me/courses", h.RegisterCourses)
			usersAPI.GET("/me/grades", h.GetGrades)
		}
		coursesAPI := API.Group("/courses")
		{
			coursesAPI.GET("", h.SearchCourses)
			coursesAPI.POST("", h.AddCourse, h.IsAdmin)
			coursesAPI.GET("/:courseID", h.GetCourseDetail)
			coursesAPI.PUT("/:courseID/status", h.SetCourseStatus, h.IsAdmin)
			coursesAPI.GET("/:courseID/classes", h.GetClasses)
			coursesAPI.POST("/:courseID/classes", h.AddClass, h.IsAdmin)
			coursesAPI.POST("/:courseID/classes/:classID/assignments", h.SubmitAssignment)
			coursesAPI.PUT("/:courseID/classes/:classID/assignments/scores", h.RegisterScores, h.IsAdmin)
			coursesAPI.GET("/:courseID/classes/:classID/assignments/export", h.DownloadSubmittedAssignments, h.IsAdmin)
		}
		announcementsAPI := API.Group("/announcements")
		{
			announcementsAPI.GET("", h.GetAnnouncementList)
			announcementsAPI.POST("", h.AddAnnouncement, h.IsAdmin)
			announcementsAPI.GET("/:announcementID", h.GetAnnouncementDetail)
		}
	}

	e.Logger.Error(e.StartServer(e.Server))
}

type InitializeResponse struct {
	Language string `json:"language"`
}

// Initialize POST /initialize 初期化エンドポイント
func (h *handlers) Initialize(c echo.Context) error {
	if err := rdb.FlushAll(c.Request().Context()).Err(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to initialize: "+err.Error())
	}

	dbForInit, _ := GetDBOtel()

	files := []string{
		"1_schema.sql",
		"2_init.sql",
		"3_sample.sql",
	}
	for _, file := range files {
		data, err := os.ReadFile(SQLDirectory + file)
		if err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
		if _, err := dbForInit.ExecContext(c.Request().Context(), string(data)); err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
	}

	if err := exec.Command("rm", "-rf", AssignmentsDirectory).Run(); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	if err := exec.Command("cp", "-r", InitDataDirectory, AssignmentsDirectory).Run(); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	type subumitterNum struct {
		ClassID    string `db:"class_id"`
		Submitters int    `db:"submitters"`
	}
	var subumitterNums []subumitterNum
	if err := dbForInit.SelectContext(c.Request().Context(), &subumitterNums, "SELECT class_id, COUNT(*) AS submitters FROM submissions GROUP BY class_id"); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	for _, sn := range subumitterNums {
		if err := rdb.Set(context.Background(), "submissions:"+sn.ClassID, sn.Submitters, time.Minute*2).Err(); err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
	}

	type totalScore struct {
		UserID     string `db:"user_id"`
		TotalScore int    `db:"total_score"`
		CourseID   string `db:"course_id"`
	}
	var totalScores []totalScore
	query := "SELECT users.id AS user_id, courses.id AS course_id, COALESCE(SUM(`submissions`.`score`), 0) AS `total_score`" +
		" FROM `users`" +
		" JOIN `registrations` ON `users`.`id` = `registrations`.`user_id`" +
		" JOIN `courses` ON `registrations`.`course_id` = `courses`.`id`" +
		" LEFT JOIN `classes` ON `courses`.`id` = `classes`.`course_id`" +
		" LEFT JOIN `submissions` ON `users`.`id` = `submissions`.`user_id` AND `submissions`.`class_id` = `classes`.`id`" +
		" GROUP BY `users`.`id`, `courses`.`id`"
	if err := h.DB.SelectContext(c.Request().Context(), &totalScores, query); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	for _, ts := range totalScores {
		if err := rdb.Set(context.Background(), "course_total_scores:"+ts.CourseID+":"+ts.UserID, ts.TotalScore, 0).Err(); err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
	}

	res := InitializeResponse{
		Language: "go",
	}
	return c.JSON(http.StatusOK, res)
}

// IsLoggedIn ログイン確認用middleware
func (h *handlers) IsLoggedIn(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		sess, err := session.Get(SessionName, c)
		if err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
		if sess.IsNew {
			return c.String(http.StatusUnauthorized, "You are not logged in.")
		}
		_, ok := sess.Values["userID"]
		if !ok {
			return c.String(http.StatusUnauthorized, "You are not logged in.")
		}

		return next(c)
	}
}

// IsAdmin admin確認用middleware
func (h *handlers) IsAdmin(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		sess, err := session.Get(SessionName, c)
		if err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
		isAdmin, ok := sess.Values["isAdmin"]
		if !ok {
			c.Logger().Error("failed to get isAdmin from session")
			return c.NoContent(http.StatusInternalServerError)
		}
		if !isAdmin.(bool) {
			return c.String(http.StatusForbidden, "You are not admin user.")
		}

		return next(c)
	}
}

func getUserInfo(c echo.Context) (userID string, userName string, isAdmin bool, err error) {
	sess, err := session.Get(SessionName, c)
	if err != nil {
		return "", "", false, err
	}
	_userID, ok := sess.Values["userID"]
	if !ok {
		return "", "", false, errors.New("failed to get userID from session")
	}
	_userName, ok := sess.Values["userName"]
	if !ok {
		return "", "", false, errors.New("failed to get userName from session")
	}
	_isAdmin, ok := sess.Values["isAdmin"]
	if !ok {
		return "", "", false, errors.New("failed to get isAdmin from session")
	}
	return _userID.(string), _userName.(string), _isAdmin.(bool), nil
}

type UserType string

const (
	Student UserType = "student"
	Teacher UserType = "teacher"
)

type User struct {
	ID             string   `db:"id"`
	Code           string   `db:"code"`
	Name           string   `db:"name"`
	HashedPassword []byte   `db:"hashed_password"`
	Type           UserType `db:"type"`
}

type CourseType string

const (
	LiberalArts   CourseType = "liberal-arts"
	MajorSubjects CourseType = "major-subjects"
)

type DayOfWeek string

const (
	Monday    DayOfWeek = "monday"
	Tuesday   DayOfWeek = "tuesday"
	Wednesday DayOfWeek = "wednesday"
	Thursday  DayOfWeek = "thursday"
	Friday    DayOfWeek = "friday"
)

var daysOfWeek = []DayOfWeek{Monday, Tuesday, Wednesday, Thursday, Friday}

type CourseStatus string

const (
	StatusRegistration CourseStatus = "registration"
	StatusInProgress   CourseStatus = "in-progress"
	StatusClosed       CourseStatus = "closed"
)

type Course struct {
	ID          string       `db:"id"`
	Code        string       `db:"code"`
	Type        CourseType   `db:"type"`
	Name        string       `db:"name"`
	Description string       `db:"description"`
	Credit      uint8        `db:"credit"`
	Period      uint8        `db:"period"`
	DayOfWeek   DayOfWeek    `db:"day_of_week"`
	TeacherID   string       `db:"teacher_id"`
	Keywords    string       `db:"keywords"`
	Status      CourseStatus `db:"status"`
}

// ---------- Public API ----------

type LoginRequest struct {
	Code     string `json:"code"`
	Password string `json:"password"`
}

// Login POST /login ログイン
func (h *handlers) Login(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return c.String(http.StatusBadRequest, "Invalid format.")
	}

	var user User
	if err := h.DB.GetContext(c.Request().Context(), &user, "SELECT * FROM `users` WHERE `code` = ?", req.Code); err != nil && err != sql.ErrNoRows {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	} else if err == sql.ErrNoRows {
		return c.String(http.StatusUnauthorized, "Code or Password is wrong.")
	}

	if bcrypt.CompareHashAndPassword(user.HashedPassword, []byte(req.Password)) != nil {
		return c.String(http.StatusUnauthorized, "Code or Password is wrong.")
	}

	sess, err := session.Get(SessionName, c)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	if userID, ok := sess.Values["userID"].(string); ok && userID == user.ID {
		return c.String(http.StatusBadRequest, "You are already logged in.")
	}

	sess.Values["userID"] = user.ID
	sess.Values["userName"] = user.Name
	sess.Values["code"] = user.Code
	sess.Values["isAdmin"] = user.Type == Teacher
	sess.Options = &sessions.Options{
		Path:   "/",
		MaxAge: 3600,
	}

	if err := sess.Save(c.Request(), c.Response()); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.NoContent(http.StatusOK)
}

// Logout POST /logout ログアウト
func (h *handlers) Logout(c echo.Context) error {
	sess, err := session.Get(SessionName, c)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	sess.Options = &sessions.Options{
		Path:   "/",
		MaxAge: -1,
	}

	if err := sess.Save(c.Request(), c.Response()); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.NoContent(http.StatusOK)
}

// ---------- Users API ----------

type GetMeResponse struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	IsAdmin bool   `json:"is_admin"`
}

// GetMe GET /api/users/me 自身の情報を取得
func (h *handlers) GetMe(c echo.Context) error {
	_, userName, isAdmin, err := getUserInfo(c)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	sess, err := session.Get(SessionName, c)
	var userCode string
	if err != nil {
		return c.NoContent(http.StatusInternalServerError)
	}

	userCode = sess.Values["code"].(string)
	// if err := h.DB.GetContext(c.Request().Context(), &userCode, "SELECT `code` FROM `users` WHERE `id` = ?", userID); err != nil {
	// 	c.Logger().Error(err)
	// 	return c.NoContent(http.StatusInternalServerError)
	// }

	return c.JSON(http.StatusOK, GetMeResponse{
		Code:    userCode,
		Name:    userName,
		IsAdmin: isAdmin,
	})
}

type GetRegisteredCourseResponseContent struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Teacher   string    `json:"teacher"`
	Period    uint8     `json:"period"`
	DayOfWeek DayOfWeek `json:"day_of_week"`
}

// GetRegisteredCourses GET /api/users/me/courses 履修中の科目一覧取得
func (h *handlers) GetRegisteredCourses(c echo.Context) error {
	userID, _, _, err := getUserInfo(c)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	//tx, err := h.DB.BeginTxx(c.Request().Context(), nil)
	//if err != nil {
	//	c.Logger().Error(err)
	//	return c.NoContent(http.StatusInternalServerError)
	//}
	//defer tx.Rollback()

	db := h.DB
	var courses []Course
	query := "SELECT `courses`.*" +
		" FROM `courses`" +
		" JOIN `registrations` ON `courses`.`id` = `registrations`.`course_id`" +
		" WHERE `courses`.`status` != ? AND `registrations`.`user_id` = ?"
	if err := db.SelectContext(c.Request().Context(), &courses, query, StatusClosed, userID); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	if len(courses) == 0 {
		return c.JSON(http.StatusOK, []GetRegisteredCourseResponseContent{})
	}

	teacherIDs := lo.Map(courses, func(course Course, _ int) string {
		return course.TeacherID
	})
	teacherIDs = lo.Uniq(teacherIDs)
	uqs, args, err := sqlx.In("SELECT * FROM users WHERE `id` IN (?)", teacherIDs)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	var teachers []User
	if err := db.SelectContext(c.Request().Context(), &teachers, uqs, args...); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	teachersMap := lo.Associate(teachers, func(teacher User) (string, User) {
		return teacher.ID, teacher
	})

	// 履修科目が0件の時は空配列を返却
	res := make([]GetRegisteredCourseResponseContent, 0, len(courses))
	for _, course := range courses {
		teacher := teachersMap[course.TeacherID]

		res = append(res, GetRegisteredCourseResponseContent{
			ID:        course.ID,
			Name:      course.Name,
			Teacher:   teacher.Name,
			Period:    course.Period,
			DayOfWeek: course.DayOfWeek,
		})
	}

	//if err := tx.Commit(); err != nil {
	//	c.Logger().Error(err)
	//	return c.NoContent(http.StatusInternalServerError)
	//}

	return c.JSON(http.StatusOK, res)
}

type RegisterCourseRequestContent struct {
	ID string `json:"id"`
}

type RegisterCoursesErrorResponse struct {
	CourseNotFound       []string `json:"course_not_found,omitempty"`
	NotRegistrableStatus []string `json:"not_registrable_status,omitempty"`
	ScheduleConflict     []string `json:"schedule_conflict,omitempty"`
}

// RegisterCourses PUT /api/users/me/courses 履修登録
func (h *handlers) RegisterCourses(c echo.Context) error {
	userID, _, _, err := getUserInfo(c)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	var req []RegisterCourseRequestContent
	if err := c.Bind(&req); err != nil {
		return c.String(http.StatusBadRequest, "Invalid format.")
	}
	sort.Slice(req, func(i, j int) bool {
		return req[i].ID < req[j].ID
	})

	tx, err := h.DB.BeginTxx(c.Request().Context(), nil)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	defer tx.Rollback()

	var errors RegisterCoursesErrorResponse
	var newlyAdded []Course

	courseIDs := make([]string, 0, len(req))
	courseIDSelectsQuerys := make([]string, 0, len(req))
	for _, courseReq := range req {
		courseIDs = append(courseIDs, courseReq.ID)
		courseIDSelectsQuerys = append(courseIDSelectsQuerys, fmt.Sprintf("('%v')", courseReq.ID))
	}

	type QueryCourse struct {
		QueryCourseID string       `db:"query_course_id"`
		ID            string       `db:"id"`
		Status        CourseStatus `db:"status"`
	}
	queryCourse := make([]QueryCourse, 0, len(req))

	// クエリの実行
	// SELECT query_course_ids.id as query_course_id, courses.* FROM (VALUES ('01FF4RXEKS0DG2EG20CYAYCCGM'), ('01FF4RXEKS0DG2EG20CWPQ60M3'), ('33333333333333333333333333')) as query_course_ids(id) LEFT JOIN isucholar.courses ON query_course_ids.id = isucholar.courses.id
	bulkQuery := "SELECT query_course_ids.query_course_id as query_course_id, case when courses.id is null then '' else courses.id end as id, case when courses.status is null then '' else courses.status end as status FROM (VALUES " + strings.Join(courseIDSelectsQuerys, ", ") + ") as query_course_ids(query_course_id) LEFT JOIN isucholar.courses ON query_course_ids.query_course_id = isucholar.courses.id"
	err = tx.SelectContext(c.Request().Context(), &queryCourse, bulkQuery)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	for _, qc := range queryCourse {
		if qc.ID == "" {
			errors.CourseNotFound = append(errors.CourseNotFound, qc.QueryCourseID)
			continue
		}

		if qc.Status != StatusRegistration {
			errors.NotRegistrableStatus = append(errors.NotRegistrableStatus, qc.ID)
			continue
		}
	}

	query, args, err := sqlx.In("with c as (select * from courses where id in (?)), r as (select * from registrations where user_id = ?) select c.* from c left join r on c.id = r.course_id where course_id is null;", courseIDs, userID)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	err = tx.SelectContext(c.Request().Context(), &newlyAdded, query, args...)
	if err == sql.ErrNoRows {
		// do nothing
	} else if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	//for _, courseReq := range req {
	//	courseID := courseReq.ID
	//	var course Course
	//	if err := tx.GetContext(c.Request().Context(), &course, "SELECT * FROM `courses` WHERE `id` = ? FOR SHARE", courseID); err != nil && err != sql.ErrNoRows {
	//		c.Logger().Error(err)
	//		return c.NoContent(http.StatusInternalServerError)
	//	} else if err == sql.ErrNoRows {
	//		errors.CourseNotFound = append(errors.CourseNotFound, courseReq.ID)
	//		continue
	//	}
	//
	//	if course.Status != StatusRegistration {
	//		errors.NotRegistrableStatus = append(errors.NotRegistrableStatus, course.ID)
	//		continue
	//	}
	//
	//	// すでに履修登録済みの科目は無視する
	//	var count int
	//	if err := tx.GetContext(c.Request().Context(), &count, "SELECT COUNT(*) FROM `registrations` WHERE `course_id` = ? AND `user_id` = ?", course.ID, userID); err != nil {
	//		c.Logger().Error(err)
	//		return c.NoContent(http.StatusInternalServerError)
	//	}
	//	if count > 0 {
	//		continue
	//	}
	//
	//	newlyAdded = append(newlyAdded, course)
	//}

	var alreadyRegistered []Course
	query = "SELECT `courses`.*" +
		" FROM `courses`" +
		" JOIN `registrations` ON `courses`.`id` = `registrations`.`course_id`" +
		" WHERE `courses`.`status` != ? AND `registrations`.`user_id` = ?"
	if err := tx.SelectContext(c.Request().Context(), &alreadyRegistered, query, StatusClosed, userID); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	alreadyRegistered = append(alreadyRegistered, newlyAdded...)
	for _, course1 := range newlyAdded {
		for _, course2 := range alreadyRegistered {
			if course1.ID != course2.ID && course1.Period == course2.Period && course1.DayOfWeek == course2.DayOfWeek {
				errors.ScheduleConflict = append(errors.ScheduleConflict, course1.ID)
				break
			}
		}
	}

	if len(errors.CourseNotFound) > 0 || len(errors.NotRegistrableStatus) > 0 || len(errors.ScheduleConflict) > 0 {
		return c.JSON(http.StatusBadRequest, errors)
	}

	newlyAddedStrs := make([]string, 0, len(newlyAdded))
	for _, course := range newlyAdded {
		newlyAddedStrs = append(newlyAddedStrs, fmt.Sprintf("('%v', '%v')", course.ID, userID))
		ctx := c.Request().Context()
		if err := rdb.Set(ctx, "course_total_scores:"+course.ID+":"+userID, 0, 0).Err(); err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
	}

	query = "INSERT INTO `registrations` (`course_id`, `user_id`) VALUES " + strings.Join(newlyAddedStrs, ", ") + " ON CONFLICT(course_id, user_id) DO NOTHING"
	_, err = tx.ExecContext(c.Request().Context(), query)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	if err = tx.Commit(); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.NoContent(http.StatusOK)
}

type Class struct {
	ID               string `db:"id"`
	CourseID         string `db:"course_id"`
	Part             uint8  `db:"part"`
	Title            string `db:"title"`
	Description      string `db:"description"`
	SubmissionClosed bool   `db:"submission_closed"`
}

type GetGradeResponse struct {
	Summary       Summary        `json:"summary"`
	CourseResults []CourseResult `json:"courses"`
}

type Summary struct {
	Credits   int     `json:"credits"`
	GPA       float64 `json:"gpa"`
	GpaTScore float64 `json:"gpa_t_score"` // 偏差値
	GpaAvg    float64 `json:"gpa_avg"`     // 平均値
	GpaMax    float64 `json:"gpa_max"`     // 最大値
	GpaMin    float64 `json:"gpa_min"`     // 最小値
}

type CourseResult struct {
	Name             string       `json:"name"`
	Code             string       `json:"code"`
	TotalScore       int          `json:"total_score"`
	TotalScoreTScore float64      `json:"total_score_t_score"` // 偏差値
	TotalScoreAvg    float64      `json:"total_score_avg"`     // 平均値
	TotalScoreMax    int          `json:"total_score_max"`     // 最大値
	TotalScoreMin    int          `json:"total_score_min"`     // 最小値
	ClassScores      []ClassScore `json:"class_scores"`
}

type ClassScore struct {
	ClassID    string `json:"class_id"`
	Title      string `json:"title"`
	Part       uint8  `json:"part"`
	Score      *int   `json:"score"`      // 0~100点
	Submitters int    `json:"submitters"` // 提出した学生数
}

// GetGrades GET /api/users/me/grades 成績取得
func (h *handlers) GetGrades(c echo.Context) error {
	userID, _, _, err := getUserInfo(c)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	// 履修している科目一覧取得
	var registeredCourses []Course
	query := "SELECT `courses`.*" +
		" FROM `registrations`" +
		" JOIN `courses` ON `registrations`.`course_id` = `courses`.`id`" +
		" WHERE `user_id` = ?"
	if err := h.DB.SelectContext(c.Request().Context(), &registeredCourses, query, userID); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	// 科目毎の成績計算処理
	courseResults := make([]CourseResult, 0, len(registeredCourses))
	myGPA := 0.0
	myCredits := 0

	courseIDs := lo.Map(registeredCourses, func(course Course, _ int) string {
		return course.ID
	})
	cqs, args, err := sqlx.In("SELECT * FROM classes WHERE course_id IN (?) ORDER BY part DESC", courseIDs)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	var classes []Class
	if err := h.DB.SelectContext(c.Request().Context(), &classes, cqs, args...); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	courseClassMap := lo.GroupBy(classes, func(class Class) string {
		return class.CourseID
	})
	type submissionScore struct {
		ClassID string        `db:"class_id"`
		Score   sql.NullInt16 `db:"score"`
	}
	classIDs := lo.Map(classes, func(class Class, _ int) string {
		return class.ID
	})
	sqs, args, err := sqlx.In("SELECT class_id, score FROM submissions WHERE class_id IN (?) AND user_id = ?", classIDs, userID)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	var submissionScores []submissionScore
	if err := h.DB.SelectContext(c.Request().Context(), &submissionScores, sqs, args...); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	classSubmissionScoreMap := lo.Associate(submissionScores, func(submissionScore submissionScore) (string, sql.NullInt16) {
		return submissionScore.ClassID, submissionScore.Score
	})
	type courseUser struct {
		UserID   string `db:"user_id"`
		CourseID string `db:"course_id"`
	}
	cuqs, args, err := sqlx.In("SELECT user_id, course_id FROM registrations WHERE course_id IN (?)", courseIDs)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	var courseUserIDs []courseUser
	if err := h.DB.SelectContext(c.Request().Context(), &courseUserIDs, cuqs, args...); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	courseUserIDsMap := lo.GroupBy(courseUserIDs, func(courseUser courseUser) string {
		return courseUser.CourseID
	})

	ctx := c.Request().Context()
	for _, course := range registeredCourses {
		classes := courseClassMap[course.ID]

		// 講義毎の成績計算処理
		classScores := make([]ClassScore, 0, len(classes))
		var myTotalScore int
		for _, class := range classes {
			// redisから提出者数取得
			submissionsCount, err := rdb.Get(ctx, "submissions:"+class.ID).Int()
			if err != nil && !errors.Is(err, redis.Nil) {
				c.Logger().Error(err)
				return c.NoContent(http.StatusInternalServerError)
			}
			score, ok := classSubmissionScoreMap[class.ID]
			if !ok || !score.Valid {
				classScores = append(classScores, ClassScore{
					ClassID:    class.ID,
					Part:       class.Part,
					Title:      class.Title,
					Score:      nil,
					Submitters: submissionsCount,
				})
			} else {
				_score := int(score.Int16)
				myTotalScore += _score
				classScores = append(classScores, ClassScore{
					ClassID:    class.ID,
					Part:       class.Part,
					Title:      class.Title,
					Score:      &_score,
					Submitters: submissionsCount,
				})
			}
		}

		// この科目を履修している学生のTotalScore一覧を取得
		//var totals []int
		//query := "SELECT COALESCE(SUM(`submissions`.`score`), 0) AS `total_score`" +
		//	" FROM `users`" +
		//	" JOIN `registrations` ON `users`.`id` = `registrations`.`user_id`" +
		//	" JOIN `courses` ON `registrations`.`course_id` = `courses`.`id`" +
		//	" LEFT JOIN `classes` ON `courses`.`id` = `classes`.`course_id`" +
		//	" LEFT JOIN `submissions` ON `users`.`id` = `submissions`.`user_id` AND `submissions`.`class_id` = `classes`.`id`" +
		//	" WHERE `courses`.`id` = ?" +
		//	" GROUP BY `users`.`id`"
		//if err := h.DB.SelectContext(c.Request().Context(), &totals, query, course.ID); err != nil {
		//	c.Logger().Error(err)
		//	return c.NoContent(http.StatusInternalServerError)
		//}
		registeredUsers := courseUserIDsMap[course.ID]
		totalKeys := lo.Map(registeredUsers, func(cu courseUser, _ int) string {
			return fmt.Sprintf("course_total_scores:%s:%s", course.ID, cu.UserID)
		})
		_totals, err := rdb.MGet(ctx, totalKeys...).Result()
		if err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
		totals := make([]int, 0, len(registeredUsers))
		for _, _total := range _totals {
			if _total == nil {
				totals = append(totals, 0)
				continue
			}
			total, err := strconv.Atoi(_total.(string))
			if err != nil {
				c.Logger().Error(err)
				return c.NoContent(http.StatusInternalServerError)
			}
			totals = append(totals, total)
		}

		courseResults = append(courseResults, CourseResult{
			Name:             course.Name,
			Code:             course.Code,
			TotalScore:       myTotalScore,
			TotalScoreTScore: tScoreInt(myTotalScore, totals),
			TotalScoreAvg:    averageInt(totals, 0),
			TotalScoreMax:    maxInt(totals, 0),
			TotalScoreMin:    minInt(totals, 0),
			ClassScores:      classScores,
		})

		// 自分のGPA計算
		if course.Status == StatusClosed {
			myGPA += float64(myTotalScore * int(course.Credit))
			myCredits += int(course.Credit)
		}
	}
	if myCredits > 0 {
		myGPA = myGPA / 100 / float64(myCredits)
	}
	now := time.Now()
	gpas := onmemoryGPAs
	if latestGPAs.IsZero() || len(gpas) == 0 || latestGPAs.Add(time.Second*3).Unix() < now.Unix() {
		gpasIf, err, _ := gpasSingleflight.Do("gpas", func() (interface{}, error) {
			// GPAの統計値
			// 一つでも修了した科目がある学生のGPA一覧
			var gpas []float64
			query = "SELECT COALESCE(SUM(submissions.score::float * courses.credit::float), 0) / 100 / credits.credits::float AS `gpa`" +
				" FROM `users`" +
				" JOIN (" +
				"     SELECT `users`.`id` AS `user_id`, SUM(`courses`.`credit`) AS `credits`" +
				"     FROM `users`" +
				"     JOIN `registrations` ON `users`.`id` = `registrations`.`user_id`" +
				"     JOIN `courses` ON `registrations`.`course_id` = `courses`.`id` AND `courses`.`status` = ?" +
				"     GROUP BY `users`.`id`" +
				" ) AS `credits` ON `credits`.`user_id` = `users`.`id`" +
				" JOIN `registrations` ON `users`.`id` = `registrations`.`user_id`" +
				" JOIN `courses` ON `registrations`.`course_id` = `courses`.`id` AND `courses`.`status` = ?" +
				" LEFT JOIN `classes` ON `courses`.`id` = `classes`.`course_id`" +
				" LEFT JOIN `submissions` ON `users`.`id` = `submissions`.`user_id` AND `submissions`.`class_id` = `classes`.`id`" +
				" WHERE `users`.`type` = ?" +
				" GROUP BY `users`.`id`, credits.credits"
			if err := h.DB.SelectContext(c.Request().Context(), &gpas, query, StatusClosed, StatusClosed, Student); err != nil {
				c.Logger().Error(err)
				return nil, c.NoContent(http.StatusInternalServerError)
			}
			latestGPAs = now
			onmemoryGPAs = gpas
			return gpas, nil
		})
		if err != nil {
			return err
		}
		gpas = gpasIf.([]float64)
		// gpasSingleflight.Forget("gpas")
	}

	res := GetGradeResponse{
		Summary: Summary{
			Credits:   myCredits,
			GPA:       myGPA,
			GpaTScore: tScoreFloat64(myGPA, gpas),
			GpaAvg:    averageFloat64(gpas, 0),
			GpaMax:    maxFloat64(gpas, 0),
			GpaMin:    minFloat64(gpas, 0),
		},
		CourseResults: courseResults,
	}

	return c.JSON(http.StatusOK, res)
}

var (
	gpasSingleflight singleflight.Group
	onmemoryGPAs     []float64
	latestGPAs       time.Time
)

// ---------- Courses API ----------

// SearchCourses GET /api/courses 科目検索
func (h *handlers) SearchCourses(c echo.Context) error {
	query := "SELECT `courses`.*, `users`.`name` AS `teacher`" +
		" FROM `courses` JOIN `users` ON `courses`.`teacher_id` = `users`.`id`" +
		" WHERE 1=1"
	var condition string
	var args []interface{}

	// 無効な検索条件はエラーを返さず無視して良い

	if courseType := c.QueryParam("type"); courseType != "" {
		condition += " AND `courses`.`type` = ?"
		args = append(args, courseType)
	}

	if credit, err := strconv.Atoi(c.QueryParam("credit")); err == nil && credit > 0 {
		condition += " AND `courses`.`credit` = ?"
		args = append(args, credit)
	}

	if teacher := c.QueryParam("teacher"); teacher != "" {
		condition += " AND `users`.`name` = ?"
		args = append(args, teacher)
	}

	if period, err := strconv.Atoi(c.QueryParam("period")); err == nil && period > 0 {
		condition += " AND `courses`.`period` = ?"
		args = append(args, period)
	}

	if dayOfWeek := c.QueryParam("day_of_week"); dayOfWeek != "" {
		condition += " AND `courses`.`day_of_week` = ?"
		args = append(args, dayOfWeek)
	}

	if keywords := c.QueryParam("keywords"); keywords != "" {
		arr := strings.Split(keywords, " ")
		var nameCondition string
		for _, keyword := range arr {
			nameCondition += " AND `courses`.`name` LIKE ?"
			args = append(args, "%"+keyword+"%")
		}
		var keywordsCondition string
		for _, keyword := range arr {
			keywordsCondition += " AND `courses`.`keywords` LIKE ?"
			args = append(args, "%"+keyword+"%")
		}
		condition += fmt.Sprintf(" AND ((1=1%s) OR (1=1%s))", nameCondition, keywordsCondition)
	}

	if status := c.QueryParam("status"); status != "" {
		condition += " AND `courses`.`status` = ?"
		args = append(args, status)
	}

	condition += " ORDER BY `courses`.`code`"

	var page int
	if c.QueryParam("page") == "" {
		page = 1
	} else {
		var err error
		page, err = strconv.Atoi(c.QueryParam("page"))
		if err != nil || page <= 0 {
			return c.String(http.StatusBadRequest, "Invalid page.")
		}
	}
	limit := 20
	offset := limit * (page - 1)

	// limitより多く上限を設定し、実際にlimitより多くレコードが取得できた場合は次のページが存在する
	condition += " LIMIT ? OFFSET ?"
	args = append(args, limit+1, offset)

	// 結果が0件の時は空配列を返却
	res := make([]GetCourseDetailResponse, 0)
	if err := h.DB.SelectContext(c.Request().Context(), &res, query+condition, args...); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	var links []string
	linkURL, err := url.Parse(c.Request().URL.Path + "?" + c.Request().URL.RawQuery)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	q := linkURL.Query()
	if page > 1 {
		q.Set("page", strconv.Itoa(page-1))
		linkURL.RawQuery = q.Encode()
		links = append(links, fmt.Sprintf("<%v>; rel=\"prev\"", linkURL))
	}
	if len(res) > limit {
		q.Set("page", strconv.Itoa(page+1))
		linkURL.RawQuery = q.Encode()
		links = append(links, fmt.Sprintf("<%v>; rel=\"next\"", linkURL))
	}
	if len(links) > 0 {
		c.Response().Header().Set("Link", strings.Join(links, ","))
	}

	if len(res) == limit+1 {
		res = res[:len(res)-1]
	}

	return c.JSON(http.StatusOK, res)
}

type AddCourseRequest struct {
	Code        string     `json:"code"`
	Type        CourseType `json:"type"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Credit      int        `json:"credit"`
	Period      int        `json:"period"`
	DayOfWeek   DayOfWeek  `json:"day_of_week"`
	Keywords    string     `json:"keywords"`
}

type AddCourseResponse struct {
	ID string `json:"id"`
}

// AddCourse POST /api/courses 新規科目登録
func (h *handlers) AddCourse(c echo.Context) error {
	userID, _, _, err := getUserInfo(c)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	var req AddCourseRequest
	if err := c.Bind(&req); err != nil {
		return c.String(http.StatusBadRequest, "Invalid format.")
	}

	if req.Type != LiberalArts && req.Type != MajorSubjects {
		return c.String(http.StatusBadRequest, "Invalid course type.")
	}
	if !contains(daysOfWeek, req.DayOfWeek) {
		return c.String(http.StatusBadRequest, "Invalid day of week.")
	}

	courseID := newULID()
	_, err = h.DB.ExecContext(c.Request().Context(), "INSERT INTO `courses` (`id`, `code`, `type`, `name`, `description`, `credit`, `period`, `day_of_week`, `teacher_id`, `keywords`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		courseID, req.Code, req.Type, req.Name, req.Description, req.Credit, req.Period, req.DayOfWeek, userID, req.Keywords)
	if err != nil {
		if pgxIsDuplicateError(err) {
			var course Course
			if err := h.DB.GetContext(c.Request().Context(), &course, "SELECT * FROM `courses` WHERE `code` = ?", req.Code); err != nil {
				c.Logger().Error(err)
				return c.NoContent(http.StatusInternalServerError)
			}
			if req.Type != course.Type || req.Name != course.Name || req.Description != course.Description || req.Credit != int(course.Credit) || req.Period != int(course.Period) || req.DayOfWeek != course.DayOfWeek || req.Keywords != course.Keywords {
				return c.String(http.StatusConflict, "A course with the same code already exists.")
			}
			return c.JSON(http.StatusCreated, AddCourseResponse{ID: course.ID})
		}
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.JSON(http.StatusCreated, AddCourseResponse{ID: courseID})
}

type GetCourseDetailResponse struct {
	ID          string       `json:"id" db:"id"`
	Code        string       `json:"code" db:"code"`
	Type        string       `json:"type" db:"type"`
	Name        string       `json:"name" db:"name"`
	Description string       `json:"description" db:"description"`
	Credit      uint8        `json:"credit" db:"credit"`
	Period      uint8        `json:"period" db:"period"`
	DayOfWeek   string       `json:"day_of_week" db:"day_of_week"`
	TeacherID   string       `json:"-" db:"teacher_id"`
	Keywords    string       `json:"keywords" db:"keywords"`
	Status      CourseStatus `json:"status" db:"status"`
	Teacher     string       `json:"teacher" db:"teacher"`
}

// GetCourseDetail GET /api/courses/:courseID 科目詳細の取得
func (h *handlers) GetCourseDetail(c echo.Context) error {
	courseID := c.Param("courseID")

	var res GetCourseDetailResponse
	query := "SELECT `courses`.*, `users`.`name` AS `teacher`" +
		" FROM `courses`" +
		" JOIN `users` ON `courses`.`teacher_id` = `users`.`id`" +
		" WHERE `courses`.`id` = ?"
	if err := h.DB.GetContext(c.Request().Context(), &res, query, courseID); err != nil && err != sql.ErrNoRows {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	} else if err == sql.ErrNoRows {
		return c.String(http.StatusNotFound, "No such course.")
	}

	return c.JSON(http.StatusOK, res)
}

type SetCourseStatusRequest struct {
	Status CourseStatus `json:"status"`
}

// SetCourseStatus PUT /api/courses/:courseID/status 科目のステータスを変更
func (h *handlers) SetCourseStatus(c echo.Context) error {
	courseID := c.Param("courseID")

	var req SetCourseStatusRequest
	if err := c.Bind(&req); err != nil {
		return c.String(http.StatusBadRequest, "Invalid format.")
	}

	//var count int
	//if err := tx.GetContext(c.Request().Context(), &count, "SELECT 1 FROM `courses` WHERE `id` = ? FOR UPDATE", courseID); errors.Is(err, sql.ErrNoRows) {
	//	return c.String(http.StatusNotFound, "No such course.")
	//} else if err != nil {
	//	c.Logger().Error(err)
	//	return c.NoContent(http.StatusInternalServerError)
	//}

	var result sql.Result
	var err error
	if result, err = h.DB.ExecContext(c.Request().Context(), "UPDATE `courses` SET `status` = ? WHERE `id` = ?", req.Status, courseID); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	if err := rdb.Del(c.Request().Context(), fmt.Sprintf("%v:%v", CourseStatusCachePrefix, courseID)).Err(); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	ra, err := result.RowsAffected()
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	if ra == 0 {
		return c.String(http.StatusNotFound, "No such course.")
	}

	return c.NoContent(http.StatusOK)
}

type ClassWithSubmitted struct {
	ID               string `db:"id"`
	CourseID         string `db:"course_id"`
	Part             uint8  `db:"part"`
	Title            string `db:"title"`
	Description      string `db:"description"`
	SubmissionClosed bool   `db:"submission_closed"`
	Submitted        bool   `db:"submitted"`
}

type GetClassResponse struct {
	ID               string `json:"id"`
	Part             uint8  `json:"part"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	SubmissionClosed bool   `json:"submission_closed"`
	Submitted        bool   `json:"submitted"`
}

// GetClasses GET /api/courses/:courseID/classes 科目に紐づく講義一覧の取得
func (h *handlers) GetClasses(c echo.Context) error {
	userID, _, _, err := getUserInfo(c)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	courseID := c.Param("courseID")

	//tx, err := h.DB.BeginTxx(c.Request().Context(), nil)
	//if err != nil {
	//	c.Logger().Error(err)
	//	return c.NoContent(http.StatusInternalServerError)
	//}
	//defer tx.Rollback()

	db := h.DB
	err = rdb.Get(c.Request().Context(), fmt.Sprintf("%v:%v", courseCachePrefix, courseID)).Err()
	if errors.Is(err, redis.Nil) {
		var count int
		if err := db.GetContext(c.Request().Context(), &count, "SELECT 1 FROM `courses` WHERE `id` = ?", courseID); errors.Is(err, sql.ErrNoRows) {
			return c.String(http.StatusNotFound, "No such course.")
		} else if err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
		if err := rdb.Set(c.Request().Context(), fmt.Sprintf("%v:%v", courseCachePrefix, courseID), count, 0).Err(); err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
	}

	var classes []ClassWithSubmitted
	query := "SELECT `classes`.*, `submissions`.`user_id` IS NOT NULL AS `submitted`" +
		" FROM `classes`" +
		" LEFT JOIN `submissions` ON `classes`.`id` = `submissions`.`class_id` AND `submissions`.`user_id` = ?" +
		" WHERE `classes`.`course_id` = ?" +
		" ORDER BY `classes`.`part`"
	if err := db.SelectContext(c.Request().Context(), &classes, query, userID, courseID); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	//if err := tx.Commit(); err != nil {
	//	c.Logger().Error(err)
	//	return c.NoContent(http.StatusInternalServerError)
	//}

	// 結果が0件の時は空配列を返却
	res := make([]GetClassResponse, 0, len(classes))
	for _, class := range classes {
		res = append(res, GetClassResponse{
			ID:               class.ID,
			Part:             class.Part,
			Title:            class.Title,
			Description:      class.Description,
			SubmissionClosed: class.SubmissionClosed,
			Submitted:        class.Submitted,
		})
	}

	return c.JSON(http.StatusOK, res)
}

type AddClassRequest struct {
	Part        uint8  `json:"part"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type AddClassResponse struct {
	ClassID string `json:"class_id"`
}

const CourseStatusCachePrefix = "couse_status"

// AddClass POST /api/courses/:courseID/classes 新規講義(&課題)追加
func (h *handlers) AddClass(c echo.Context) error {
	courseID := c.Param("courseID")

	var req AddClassRequest
	if err := c.Bind(&req); err != nil {
		return c.String(http.StatusBadRequest, "Invalid format.")
	}

	//tx, err := h.DB.BeginTxx(c.Request().Context(), nil)
	//if err != nil {
	//	c.Logger().Error(err)
	//	return c.NoContent(http.StatusInternalServerError)
	//}
	//defer tx.Rollback()

	db := h.DB
	var course Course
	res, err := rdb.Get(c.Request().Context(), fmt.Sprintf("%v:%v", CourseStatusCachePrefix, courseID)).Result()
	if errors.Is(err, redis.Nil) {
		if err := db.GetContext(c.Request().Context(), &course, "SELECT * FROM `courses` WHERE `id` = ?", courseID); err != nil && err != sql.ErrNoRows {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		} else if err == sql.ErrNoRows {
			return c.String(http.StatusNotFound, "No such course.")
		}
		err := rdb.Set(c.Request().Context(), fmt.Sprintf("%v:%v", CourseStatusCachePrefix, courseID), string(course.Status), 0).Err()
		if err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
	} else if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	} else {
		course.Status = CourseStatus(res)
	}
	if course.Status != StatusInProgress {
		return c.String(http.StatusBadRequest, "This course is not in-progress.")
	}

	classID := newULID()
	if _, err := db.ExecContext(c.Request().Context(), "INSERT INTO `classes` (`id`, `course_id`, `part`, `title`, `description`) VALUES (?, ?, ?, ?, ?)",
		classID, courseID, req.Part, req.Title, req.Description); err != nil {
		//_ = tx.Rollback()
		if pgxIsDuplicateError(err) {
			var class Class
			if err := db.GetContext(c.Request().Context(), &class, "SELECT * FROM `classes` WHERE `course_id` = ? AND `part` = ?", courseID, req.Part); err != nil {
				c.Logger().Error(err)
				return c.NoContent(http.StatusInternalServerError)
			}
			if req.Title != class.Title || req.Description != class.Description {
				return c.String(http.StatusConflict, "A class with the same part already exists.")
			}
			return c.JSON(http.StatusCreated, AddClassResponse{ClassID: class.ID})
		}
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	//if err := tx.Commit(); err != nil {
	//	c.Logger().Error(err)
	//	return c.NoContent(http.StatusInternalServerError)
	//}

	return c.JSON(http.StatusCreated, AddClassResponse{ClassID: classID})
}

// SubmitAssignment POST /api/courses/:courseID/classes/:classID/assignments 課題の提出
func (h *handlers) SubmitAssignment(c echo.Context) error {
	userID, _, _, err := getUserInfo(c)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	courseID := c.Param("courseID")
	classID := c.Param("classID")

	tx, err := h.DB.BeginTxx(c.Request().Context(), nil)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	defer tx.Rollback()

	var status CourseStatus
	if err := tx.GetContext(c.Request().Context(), &status, "SELECT `status` FROM `courses` WHERE `id` = ?", courseID); err != nil && err != sql.ErrNoRows {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	} else if err == sql.ErrNoRows {
		return c.String(http.StatusNotFound, "No such course.")
	}
	if status != StatusInProgress {
		return c.String(http.StatusBadRequest, "This course is not in progress.")
	}

	//var registrationCount int
	//if err := tx.GetContext(c.Request().Context(), &registrationCount, "SELECT COUNT(*) FROM `registrations` WHERE `user_id` = ? AND `course_id` = ?", userID, courseID); err != nil {
	//	c.Logger().Error(err)
	//	return c.NoContent(http.StatusInternalServerError)
	//}

	err = rdb.Get(c.Request().Context(), fmt.Sprintf("%v:%v:%v", getAnnouncementRegistrationsCachePrefix, courseID, userID)).Err()
	if errors.Is(err, redis.Nil) {
		var registrationCount int
		if err := tx.GetContext(c.Request().Context(), &registrationCount, "SELECT 1 FROM `registrations` WHERE `course_id` = ? AND `user_id` = ?", courseID, userID); errors.Is(err, sql.ErrNoRows) {
			return c.String(http.StatusBadRequest, "You have not taken this course.")
		} else if err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
		if err = rdb.Set(c.Request().Context(), fmt.Sprintf("%v:%v:%v", getAnnouncementRegistrationsCachePrefix, courseID, userID), registrationCount, 0).Err(); err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
	} else if err != nil {
		return c.NoContent(http.StatusInternalServerError)
	}

	var submissionClosed bool
	if err := tx.GetContext(c.Request().Context(), &submissionClosed, "SELECT `submission_closed` FROM `classes` WHERE `id` = ?", classID); err != nil && err != sql.ErrNoRows {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	} else if err == sql.ErrNoRows {
		return c.String(http.StatusNotFound, "No such class.")
	}
	if submissionClosed {
		return c.String(http.StatusBadRequest, "Submission has been closed for this class.")
	}

	file, header, err := c.Request().FormFile("file")
	if err != nil {
		return c.String(http.StatusBadRequest, "Invalid file.")
	}
	defer file.Close()

	if _, err := tx.ExecContext(c.Request().Context(), "INSERT INTO `submissions` (`user_id`, `class_id`, `file_name`) VALUES (?, ?, ?) ON CONFLICT(user_id, class_id) DO UPDATE SET `file_name` = EXCLUDED.file_name", userID, classID, header.Filename); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	data, err := io.ReadAll(file)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	dst := AssignmentsDirectory + classID + "-" + userID + ".pdf"
	if err := os.WriteFile(dst, data, 0o666); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	if err := tx.Commit(); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	ctx := c.Request().Context()
	if err := rdb.Incr(ctx, "submissions:"+classID).Err(); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.NoContent(http.StatusNoContent)
}

type Score struct {
	UserCode string `json:"user_code"`
	Score    int    `json:"score"`
}

// RegisterScores PUT /api/courses/:courseID/classes/:classID/assignments/scores 採点結果登録
func (h *handlers) RegisterScores(c echo.Context) error {
	classID := c.Param("classID")

	//tx, err := h.DB.BeginTxx(c.Request().Context(), nil)
	//if err != nil {
	//	c.Logger().Error(err)
	//	return c.NoContent(http.StatusInternalServerError)
	//}
	//defer tx.Rollback()

	db := h.DB
	type class struct {
		SubmissionClosed bool   `db:"submission_closed"`
		CourseID         string `db:"course_id"`
	}
	var cls class
	if err := db.GetContext(c.Request().Context(), &cls, "SELECT course_id, `submission_closed` FROM `classes` WHERE `id` = ?", classID); err != nil && err != sql.ErrNoRows {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	} else if err == sql.ErrNoRows {
		return c.String(http.StatusNotFound, "No such class.")
	}

	if !cls.SubmissionClosed {
		return c.String(http.StatusBadRequest, "This assignment is not closed yet.")
	}

	var req []Score
	if err := c.Bind(&req); err != nil {
		return c.String(http.StatusBadRequest, "Invalid format.")
	}

	if len(req) > 0 {
		userCodes := lo.Map(req, func(score Score, _ int) string {
			return score.UserCode
		})
		uqs, args, err := sqlx.In("SELECT id, code FROM users  WHERE code IN (?)", userCodes)
		if err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
		var users []User
		if err := db.SelectContext(c.Request().Context(), &users, uqs, args...); err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
		type submissionUpdates struct {
			UserID  string `db:"user_id"`
			Score   int    `db:"score"`
			ClassID string `db:"class_id"`
		}
		userMap := lo.Associate(users, func(user User) (string, string) {
			return user.Code, user.ID
		})
		updates := lo.Map(req, func(score Score, _ int) submissionUpdates {
			uid := userMap[score.UserCode]
			rdb.IncrBy(c.Request().Context(), "course_total_scores:"+cls.CourseID+":"+uid, int64(score.Score))

			return submissionUpdates{
				UserID:  uid,
				Score:   score.Score,
				ClassID: classID,
			}
		})
		ctx := c.Request().Context()
		if _, err := db.NamedExecContext(ctx, "INSERT INTO `submissions` (`user_id`, `class_id`, `score`, `file_name`) VALUES (:user_id, :class_id, :score, '') ON CONFLICT(user_id, class_id) DO UPDATE SET `score` = EXCLUDED.score", updates); err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
	}

	//if err := tx.Commit(); err != nil {
	//	c.Logger().Error(err)
	//	return c.NoContent(http.StatusInternalServerError)
	//}

	return c.NoContent(http.StatusNoContent)
}

type Submission struct {
	UserID   string `db:"user_id"`
	UserCode string `db:"user_code"`
	FileName string `db:"file_name"`
}

// DownloadSubmittedAssignments GET /api/courses/:courseID/classes/:classID/assignments/export 提出済みの課題ファイルをzip形式で一括ダウンロード
func (h *handlers) DownloadSubmittedAssignments(c echo.Context) error {
	classID := c.Param("classID")

	tx, err := h.DB.BeginTxx(c.Request().Context(), nil)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	defer tx.Rollback()

	var classCount int
	if err := tx.GetContext(c.Request().Context(), &classCount, "SELECT 1 FROM `classes` WHERE `id` = ? FOR UPDATE", classID); errors.Is(err, sql.ErrNoRows) {
		return c.String(http.StatusNotFound, "No such class.")
	} else if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	var submissions []Submission
	query := "SELECT `submissions`.`user_id`, `submissions`.`file_name`, `users`.`code` AS `user_code`" +
		" FROM `submissions`" +
		" JOIN `users` ON `users`.`id` = `submissions`.`user_id`" +
		" WHERE `class_id` = ?"
	if err := tx.SelectContext(c.Request().Context(), &submissions, query, classID); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	zipFilePath := AssignmentsDirectory + classID + ".zip"
	//if err := createSubmissionsZip(zipFilePath, classID, submissions); err != nil {
	//	c.Logger().Error(err)
	//	return c.NoContent(http.StatusInternalServerError)
	//}

	buf, err := createSubmissionsZipOnMemory(zipFilePath, classID, submissions)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	if _, err := tx.ExecContext(c.Request().Context(), "UPDATE `classes` SET `submission_closed` = true WHERE `id` = ?", classID); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	if err := tx.Commit(); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.Blob(200, "application/zip", buf)
}

//func createSubmissionsZip(zipFilePath string, classID string, submissions []Submission) error {
//	tmpDir := AssignmentsDirectory + classID + "/"
//	if err := exec.Command("rm", "-rf", tmpDir).Run(); err != nil {
//		return err
//	}
//	if err := exec.Command("mkdir", tmpDir).Run(); err != nil {
//		return err
//	}
//
//	// ファイル名を指定の形式に変更
//	for _, submission := range submissions {
//		if err := exec.Command(
//			"cp",
//			AssignmentsDirectory+classID+"-"+submission.UserID+".pdf",
//			tmpDir+submission.UserCode+"-"+submission.FileName,
//		).Run(); err != nil {
//			return err
//		}
//	}
//
//	// -i 'tmpDir/*': 空zipを許す
//	return exec.Command("zip", "-j", "-r", zipFilePath, tmpDir, "-i", tmpDir+"*").Run()
//}

func createSubmissionsZipOnMemory(zipFilePath string, classID string, submissions []Submission) ([]byte, error) {
	//tmpDir := AssignmentsDirectory + classID + "/"
	//if err := exec.Command("rm", "-rf", tmpDir).Run(); err != nil {
	//	return nil, err
	//}
	//if err := exec.Command("mkdir", tmpDir).Run(); err != nil {
	//	return nil, err
	//}

	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	// ファイル名を指定の形式に変更
	for _, submission := range submissions {
		f, err := os.Open(AssignmentsDirectory + classID + "-" + submission.UserID + ".pdf")
		if err != nil {
			return nil, err
		}

		// ZIPファイルにファイルエントリを追加
		zipFile, err := zw.CreateHeader(
			&zip.FileHeader{
				Name:   submission.UserCode + "-" + submission.FileName,
				Method: zip.Store,
			},
		)
		if err != nil {
			return nil, err
		}

		// ファイルの内容をZIPにコピー
		_, err = io.Copy(zipFile, f)
		if err != nil {
			return nil, err
		}

		//if err := exec.Command(
		//	"cp",
		//	AssignmentsDirectory+classID+"-"+submission.UserID+".pdf",
		//	tmpDir+submission.UserCode+"-"+submission.FileName,
		//).Run(); err != nil {
		//	return err
		//}

		f.Close()
	}

	err := zw.Close()
	if err != nil {
		return nil, err
	}

	// -i 'tmpDir/*': 空zipを許す
	// return exec.Command("zip", "-j", "-r", zipFilePath, tmpDir, "-i", tmpDir+"*").Run()
	return buf.Bytes(), nil
}

// ---------- Announcement API ----------

type AnnouncementWithoutDetail struct {
	ID         string `json:"id" db:"id"`
	CourseID   string `json:"course_id" db:"course_id"`
	CourseName string `json:"course_name" db:"course_name"`
	Title      string `json:"title" db:"title"`
	Unread     bool   `json:"unread" db:"unread"`
}

type GetAnnouncementsResponse struct {
	UnreadCount   int                         `json:"unread_count"`
	Announcements []AnnouncementWithoutDetail `json:"announcements"`
}

// GetAnnouncementList GET /api/announcements お知らせ一覧取得
func (h *handlers) GetAnnouncementList(c echo.Context) error {
	userID, _, _, err := getUserInfo(c)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	//tx, err := h.DB.BeginTxx(c.Request().Context(), nil)
	//if err != nil {
	//	c.Logger().Error(err)
	//	return c.NoContent(http.StatusInternalServerError)
	//}
	//defer tx.Rollback()

	db := h.DB
	var announcements []AnnouncementWithoutDetail
	var args []interface{}
	query := "SELECT `announcements`.`id`, `courses`.`id` AS `course_id`, `courses`.`name` AS `course_name`, `announcements`.`title`, COALESCE(NOT unread_announcements.is_deleted, false) AS unread" +
		" FROM `announcements`" +
		" JOIN `courses` ON `announcements`.`course_id` = `courses`.`id`" +
		" JOIN `registrations` ON `courses`.`id` = `registrations`.`course_id`" +
		" LEFT JOIN `unread_announcements` ON `announcements`.`id` = `unread_announcements`.`announcement_id` AND unread_announcements.user_id = ?" +
		" WHERE 1=1"
	args = append(args, userID)

	if courseID := c.QueryParam("course_id"); courseID != "" {
		query += " AND `announcements`.`course_id` = ?"
		args = append(args, courseID)
	}

	query += " AND `registrations`.`user_id` = ?" +
		" ORDER BY `announcements`.`id` DESC" +
		" LIMIT ? OFFSET ?"
	args = append(args, userID)

	var page int
	if c.QueryParam("page") == "" {
		page = 1
	} else {
		page, err = strconv.Atoi(c.QueryParam("page"))
		if err != nil || page <= 0 {
			return c.String(http.StatusBadRequest, "Invalid page.")
		}
	}
	limit := 20
	offset := limit * (page - 1)
	// limitより多く上限を設定し、実際にlimitより多くレコードが取得できた場合は次のページが存在する
	args = append(args, limit+1, offset)

	if err := db.SelectContext(c.Request().Context(), &announcements, query, args...); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	var unreadCount int
	if err := db.GetContext(c.Request().Context(), &unreadCount, "SELECT COUNT(*) FROM `unread_announcements` WHERE `user_id` = ?", userID); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	//if err := tx.Commit(); err != nil {
	//	c.Logger().Error(err)
	//	return c.NoContent(http.StatusInternalServerError)
	//}

	var links []string
	linkURL, err := url.Parse(c.Request().URL.Path + "?" + c.Request().URL.RawQuery)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	q := linkURL.Query()
	if page > 1 {
		q.Set("page", strconv.Itoa(page-1))
		linkURL.RawQuery = q.Encode()
		links = append(links, fmt.Sprintf("<%v>; rel=\"prev\"", linkURL))
	}
	if len(announcements) > limit {
		q.Set("page", strconv.Itoa(page+1))
		linkURL.RawQuery = q.Encode()
		links = append(links, fmt.Sprintf("<%v>; rel=\"next\"", linkURL))
	}
	if len(links) > 0 {
		c.Response().Header().Set("Link", strings.Join(links, ","))
	}

	if len(announcements) == limit+1 {
		announcements = announcements[:len(announcements)-1]
	}

	// 対象になっているお知らせが0件の時は空配列を返却
	announcementsRes := append(make([]AnnouncementWithoutDetail, 0, len(announcements)), announcements...)

	return c.JSON(http.StatusOK, GetAnnouncementsResponse{
		UnreadCount:   unreadCount,
		Announcements: announcementsRes,
	})
}

type Announcement struct {
	ID       string `db:"id"`
	CourseID string `db:"course_id"`
	Title    string `db:"title"`
	Message  string `db:"message"`
}

type AddAnnouncementRequest struct {
	ID       string `json:"id"`
	CourseID string `json:"course_id"`
	Title    string `json:"title"`
	Message  string `json:"message"`
}

const courseCachePrefix = "course"

// AddAnnouncement POST /api/announcements 新規お知らせ追加
func (h *handlers) AddAnnouncement(c echo.Context) error {
	var req AddAnnouncementRequest
	if err := c.Bind(&req); err != nil {
		return c.String(http.StatusBadRequest, "Invalid format.")
	}

	tx, err := h.DB.BeginTxx(c.Request().Context(), nil)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	defer tx.Rollback()

	err = rdb.Get(c.Request().Context(), fmt.Sprintf("%v:%v", courseCachePrefix, req.CourseID)).Err()
	if errors.Is(err, redis.Nil) {
		var count int
		if err := tx.GetContext(c.Request().Context(), &count, "SELECT 1 FROM `courses` WHERE `id` = ?", req.CourseID); errors.Is(err, sql.ErrNoRows) {
			return c.String(http.StatusNotFound, "No such course.")
		} else if err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
		if err := rdb.Set(c.Request().Context(), fmt.Sprintf("%v:%v", courseCachePrefix, req.CourseID), count, 0).Err(); err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
	}

	if _, err := tx.ExecContext(c.Request().Context(), "INSERT INTO `announcements` (`id`, `course_id`, `title`, `message`) VALUES (?, ?, ?, ?)",
		req.ID, req.CourseID, req.Title, req.Message); err != nil {
		_ = tx.Rollback()
		if pgxIsDuplicateError(err) {
			var announcement Announcement
			if err := h.DB.GetContext(c.Request().Context(), &announcement, "SELECT * FROM `announcements` WHERE `id` = ?", req.ID); err != nil {
				c.Logger().Error(err)
				return c.NoContent(http.StatusInternalServerError)
			}
			if announcement.CourseID != req.CourseID || announcement.Title != req.Title || announcement.Message != req.Message {
				return c.String(http.StatusConflict, "An announcement with the same id already exists.")
			}
			return c.NoContent(http.StatusCreated)
		}
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	var targets []User
	query := "SELECT `users`.* FROM `users`" +
		" JOIN `registrations` ON `users`.`id` = `registrations`.`user_id`" +
		" WHERE `registrations`.`course_id` = ?"
	if err := tx.SelectContext(c.Request().Context(), &targets, query, req.CourseID); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	if len(targets) > 0 {
		type unreadInsert struct {
			AnnouncementID string `db:"announcement_id"`
			UserID         string `db:"user_id"`
		}
		unreadInserts := lo.Map(targets, func(user User, _ int) unreadInsert {
			return unreadInsert{
				AnnouncementID: req.ID,
				UserID:         user.ID,
			}
		})
		if _, err := tx.NamedExecContext(c.Request().Context(), "INSERT INTO `unread_announcements` (`announcement_id`, `user_id`) VALUES (:announcement_id, :user_id)", unreadInserts); err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
	}

	if err := tx.Commit(); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.NoContent(http.StatusCreated)
}

type AnnouncementDetail struct {
	ID         string `json:"id" db:"id"`
	CourseID   string `json:"course_id" db:"course_id"`
	CourseName string `json:"course_name" db:"course_name"`
	Title      string `json:"title" db:"title"`
	Message    string `json:"message" db:"message"`
	Unread     bool   `json:"unread" db:"unread"`
}

const getAnnouncementRegistrationsCachePrefix = "get_announcement_registrations:"

// GetAnnouncementDetail GET /api/announcements/:announcementID お知らせ詳細取得
func (h *handlers) GetAnnouncementDetail(c echo.Context) error {
	userID, _, _, err := getUserInfo(c)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	announcementID := c.Param("announcementID")

	var announcement AnnouncementDetail
	query := "SELECT `announcements`.`id`, `courses`.`id` AS `course_id`, `courses`.`name` AS `course_name`, `announcements`.`title`, `announcements`.`message`, COALESCE(NOT unread_announcements.is_deleted, false) AS unread" +
		" FROM `announcements`" +
		" JOIN `courses` ON `courses`.`id` = `announcements`.`course_id`" +
		" LEFT JOIN `unread_announcements` ON `unread_announcements`.`announcement_id` = `announcements`.`id` AND unread_announcements.user_id = ?" +
		" WHERE `announcements`.`id` = ?"
	if err := h.DB.GetContext(c.Request().Context(), &announcement, query, userID, announcementID); err != nil && err != sql.ErrNoRows {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	} else if err == sql.ErrNoRows {
		return c.String(http.StatusNotFound, "No such announcement.")
	}

	err = rdb.Get(c.Request().Context(), fmt.Sprintf("%v:%v:%v", getAnnouncementRegistrationsCachePrefix, announcement.CourseID, userID)).Err()
	if errors.Is(err, redis.Nil) {
		var registrationCount int
		if err := h.DB.GetContext(c.Request().Context(), &registrationCount, "SELECT 1 FROM `registrations` WHERE `course_id` = ? AND `user_id` = ?", announcement.CourseID, userID); errors.Is(err, sql.ErrNoRows) {
			return c.String(http.StatusNotFound, "No such announcement.")
		} else if err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
		if err := rdb.Set(c.Request().Context(), fmt.Sprintf("%v:%v:%v", getAnnouncementRegistrationsCachePrefix, announcement.CourseID, userID), registrationCount, 0).Err(); err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
	} else if err != nil {
		return c.NoContent(http.StatusInternalServerError)
	}

	if _, err := h.DB.ExecContext(c.Request().Context(), "DELETE FROM `unread_announcements` WHERE `announcement_id` = ? AND `user_id` = ?", announcementID, userID); err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.JSON(http.StatusOK, announcement)
}
