package main

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type YorozuyaCredentials struct {
	CompanyCode string `json:"companycd"`
	Username    string `json:"username"`
	Password    string `json:"password"`
}

type YorozuyaStatus struct {
	Message   string `json:"message"`
	StartTime string `json:"startTime"`
	LeaveTime string `json:"leaveTime"`
}

func TimeRecorderToggle(w http.ResponseWriter, r *http.Request) {
	var msg YorozuyaCredentials
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		respond(w, http.StatusBadRequest, simpleMsg{Message: "invalid request body"})
		return
	}

	s, err := NewYorozuyaSession()
	if err != nil {
		respond(w, http.StatusInternalServerError, simpleMsg{Message: "Internal server error"})
		slog.Error("session error", "err", err)
		return
	}
	err = s.Login(&msg)
	if err != nil {
		respond(w, http.StatusInternalServerError, simpleMsg{Message: "Login error: " + err.Error()})
		slog.Error("login error", "err", err)
		return
	}

	switch s.state {
	case timeRecorderStateOff:
		err = s.TimeRecord(timeRecorderToOn)
		if err != nil {
			respond(w, http.StatusInternalServerError, simpleMsg{Message: "Internal server error: " + err.Error()})
			return
		}
	case timeRecorderStateOn:
		err = s.TimeRecord(timeRecorderToOff)
		if err != nil {
			respond(w, http.StatusInternalServerError, simpleMsg{Message: "Internal server error: " + err.Error()})
			return
		}
	case timeRecorderStateEnd:
		respond(w, http.StatusBadRequest, simpleMsg{Message: "本日は退勤済みです"})
		return
	case timeRecorderStateUnknown:
		respond(w, http.StatusInternalServerError, simpleMsg{Message: "unknown state: buttons not found"})
		slog.Error("unknown state: buttons not found")
		return
	}

	respond(w, http.StatusOK, YorozuyaStatus{
		Message:   "OK",
		StartTime: s.startTime,
		LeaveTime: s.leaveTime,
	})
}

func TimeRecorderStatus(w http.ResponseWriter, r *http.Request) {
	var msg YorozuyaCredentials
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		respond(w, http.StatusBadRequest, simpleMsg{Message: "invalid request body"})
		return
	}

	s, err := NewYorozuyaSession()
	if err != nil {
		respond(w, http.StatusInternalServerError, simpleMsg{Message: "Internal server error"})
		slog.Error("session error", "err", err)
		return
	}
	err = s.Login(&msg)
	if err != nil {
		respond(w, http.StatusInternalServerError, simpleMsg{Message: "Login error: " + err.Error()})
		slog.Error("login error", "err", err)
		return
	}

	respond(w, http.StatusOK, YorozuyaStatus{
		Message:   "OK",
		StartTime: s.startTime,
		LeaveTime: s.leaveTime,
	})
}

const (
	YorozuyaBaseURL = "https://www.e4628.jp"
)

type timeRecorderState int

const (
	timeRecorderStateUnknown timeRecorderState = iota
	timeRecorderStateOff                       // 出社前
	timeRecorderStateOn                        // 出社後
	timeRecorderStateEnd                       // 退勤後
)

type YorozuyaSession struct {
	client *http.Client

	csrfKey   string
	csrfValue string

	startTime string
	leaveTime string
	state     timeRecorderState
}

func NewYorozuyaSession() (*YorozuyaSession, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &YorozuyaSession{
		client: &http.Client{Jar: jar},
	}, nil
}

type m = map[string]string

func (s *YorozuyaSession) post(obj map[string]string) (string, error) {
	form := make(url.Values)
	for k, v := range obj {
		form.Set(k, v)
	}
	resp, err := s.client.PostForm(YorozuyaBaseURL, form)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

var (
	authorizedText        = `<div class="user_name">`
	csrfRegexp            = regexp.MustCompile(`name="(__sectag_[0-9a-f]+)" value="([0-9a-f]+)"`)
	timeRecordStartRegexp = regexp.MustCompile(`>出社<br\s*/?>\((\d{2}:\d{2})\)`)
	timeRecordLeaveRegexp = regexp.MustCompile(`>退社<br\s*/?>\((\d{2}:\d{2})\)`)
)

func (s *YorozuyaSession) parse(body string) error {
	// NOTE: Yorozuya returns 200 even in case authentication failed
	if !strings.Contains(body, authorizedText) {
		return errors.New("unauthorized")
	}

	match := csrfRegexp.FindStringSubmatch(body)
	if match != nil {
		s.csrfKey = match[1]
		s.csrfValue = match[2]
	} else {
		slog.Warn("csrf token not found in body")
	}

	startMatch := timeRecordStartRegexp.FindStringSubmatch(body)
	if startMatch != nil {
		s.startTime = startMatch[1]
	}
	leaveMatch := timeRecordLeaveRegexp.FindStringSubmatch(body)
	if leaveMatch != nil {
		s.leaveTime = leaveMatch[1]
	}
	if s.startTime == "" && s.leaveTime == "" {
		s.state = timeRecorderStateOff
	}
	if s.startTime != "" && s.leaveTime == "" {
		s.state = timeRecorderStateOn
	}
	if s.startTime != "" && s.leaveTime != "" {
		s.state = timeRecorderStateEnd
	}
	if s.startTime == "" && s.leaveTime != "" {
		// should be unreachable?
		s.state = timeRecorderStateUnknown
		slog.Warn("unknown state: start time not found, but end time found")
	}

	return nil
}

func (s *YorozuyaSession) Login(c *YorozuyaCredentials) error {
	body, err := s.post(m{
		"y_companycd": c.CompanyCode,
		"y_logincd":   c.Username,
		"password":    c.Password,
		"Submit":      "Login",
		"module":      "login",
		"trycnt":      "1",
	})
	if err != nil {
		return err
	}
	return s.parse(body)
}

type timeRecorderType int

var (
	timeRecorderToOn  timeRecorderType = 1
	timeRecorderToOff timeRecorderType = 2
)

func (s *YorozuyaSession) TimeRecord(typ timeRecorderType) error {
	body, err := s.post(m{
		"module":                     "timerecorder",
		"action":                     "timerecorder",
		s.csrfKey:                    s.csrfValue,
		"timerecorder_stamping_type": strconv.Itoa(int(typ)),
	})
	if err != nil {
		return err
	}
	return s.parse(body)
}
