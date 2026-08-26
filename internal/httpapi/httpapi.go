package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"seedvault/internal/application"
	"seedvault/internal/domain"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Server struct{ App *application.App }

func New(a *application.App) *Server { return &Server{App: a} }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/readyz", s.health)
	mux.HandleFunc("/api/v1/batches", s.batches)
	mux.HandleFunc("/api/v1/batches/", s.batch)
	return WithRequestTracking(mux)
}

func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func errw(w http.ResponseWriter, err error) {
	code := http.StatusBadRequest
	if strings.Contains(err.Error(), "not_found") {
		code = http.StatusNotFound
	}
	if strings.Contains(err.Error(), "revision_conflict") || strings.Contains(err.Error(), "read_only") || errors.Is(err, domain.ErrInvalidState) {
		code = http.StatusConflict
	}
	out := map[string]any{"error": err.Error()}
	var validation domain.ValidationError
	if errors.As(err, &validation) {
		out["error"], out["details"] = validation.Code, validation.Details
	}
	write(w, code, out)
}

func decode(r *http.Request, value any) error {
	r.Body = io.NopCloser(io.LimitReader(r.Body, 1<<20))
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}

func decodeBytes(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	write(w, http.StatusOK, map[string]string{"status": "ok"})
}

type createReq struct {
	Title       string                 `json:"title"`
	CustodianID string                 `json:"custodian_id"`
	Items       []domain.AccessionItem `json:"items"`
	RequestID   string                 `json:"request_id"`
}

func (s *Server) batches(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.listBatches(w, r)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var request createReq
	if err := decode(r, &request); err != nil {
		errw(w, err)
		return
	}
	if request.RequestID != "" {
		if old, ok := s.App.Store.Idem(request.RequestID); ok {
			write(w, http.StatusCreated, old)
			return
		}
	}
	b, err := s.App.Create(request.Title, request.CustodianID, request.Items)
	if err != nil {
		errw(w, err)
		return
	}
	if request.RequestID != "" {
		s.App.Store.PutIdem(request.RequestID, b)
	}
	write(w, http.StatusCreated, b)
}

func (s *Server) listBatches(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	state, custodian := query.Get("state"), query.Get("custodian_id")
	if state != "" {
		valid := map[string]bool{"draft": true, "identity_verified": true, "trial_active": true, "trial_review": true, "remediation_active": true, "approved": true, "archived": true}
		if !valid[state] {
			errw(w, errors.New("state_invalid"))
			return
		}
	}
	var after, before time.Time
	var err error
	if query.Get("created_after") != "" {
		after, err = time.Parse(time.RFC3339, query.Get("created_after"))
		if err != nil {
			errw(w, errors.New("created_after_invalid"))
			return
		}
	}
	if query.Get("created_before") != "" {
		before, err = time.Parse(time.RFC3339, query.Get("created_before"))
		if err != nil {
			errw(w, errors.New("created_before_invalid"))
			return
		}
	}
	all := s.App.List()
	sort.Slice(all, func(i, j int) bool { return all[i].BatchID < all[j].BatchID })
	filtered := make([]*domain.RejuvenationBatch, 0)
	for _, b := range all {
		if state != "" && string(b.State) != state || custodian != "" && b.CustodianID != custodian || !after.IsZero() && b.CreatedAt.Before(after) || !before.IsZero() && b.CreatedAt.After(before) {
			continue
		}
		filtered = append(filtered, b)
	}
	page := 20
	if raw := query.Get("page_size"); raw != "" {
		n, parseErr := strconv.Atoi(raw)
		if parseErr != nil || n < 1 || n > 100 {
			errw(w, errors.New("page_size_invalid"))
			return
		}
		page = n
	}
	start := 0
	if cursor := query.Get("cursor"); cursor != "" {
		for i, b := range filtered {
			if b.BatchID == cursor {
				start = i + 1
			}
		}
	}
	end := start + page
	if end > len(filtered) {
		end = len(filtered)
	}
	items := make([]map[string]any, 0, end-start)
	for _, b := range filtered[start:end] {
		items = append(items, map[string]any{"batch": b, "batch_id": b.BatchID, "state": b.State, "item_count": len(b.Items), "observation_count": len(b.Observations), "minimum_viability": minMetric(b), "active_remediation_count": activeCases(b)})
	}
	out := map[string]any{"items": items}
	if end < len(filtered) {
		out["next_cursor"] = filtered[end-1].BatchID
	}
	write(w, http.StatusOK, out)
}

func minMetric(b *domain.RejuvenationBatch) float64 {
	minimum := 101.0
	for _, item := range b.Items {
		if value := b.Metric(item.AccessionID); value < minimum {
			minimum = value
		}
	}
	if minimum == 101 {
		return 0
	}
	return minimum
}

func activeCases(b *domain.RejuvenationBatch) int {
	count := 0
	for _, remediation := range b.Remediations {
		if remediation.Resolution == "" {
			count++
		}
	}
	return count
}

func rev(r *http.Request) int {
	raw := r.Header.Get("X-Expected-Revision")
	if raw == "" {
		return -1
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return -1
	}
	return value
}

func requestRevision(r *http.Request, body *int) int {
	if body != nil {
		return *body
	}
	return rev(r)
}

type identityReq struct {
	Evidence         json.RawMessage `json:"evidence"`
	ConfirmedBy      []string        `json:"confirmed_by"`
	ExpectedRevision *int            `json:"expected_revision,omitempty"`
}

type observationReq struct {
	Observations     []domain.TrialObservation `json:"observations,omitempty"`
	ExpectedRevision *int                      `json:"expected_revision,omitempty"`
	DayIndex         int                       `json:"day_index,omitempty"`
	AccessionID      string                    `json:"accession_id,omitempty"`
	ObservedBy       string                    `json:"observed_by,omitempty"`
	GerminatedCount  int                       `json:"germinated_count,omitempty"`
	DormantCount     int                       `json:"dormant_count,omitempty"`
	MoldedCount      int                       `json:"molded_count,omitempty"`
	DeadCount        int                       `json:"dead_count,omitempty"`
}

type correctionReq struct {
	Correction       domain.TrialObservation `json:"correction"`
	Reason           string                  `json:"reason"`
	OperatorID       string                  `json:"operator_id"`
	ExpectedRevision *int                    `json:"expected_revision,omitempty"`
}

func (s *Server) batch(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		write(w, http.StatusNotFound, nil)
		return
	}
	id := parts[3]
	if len(parts) == 4 {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		b, err := s.App.Get(id)
		if err != nil {
			errw(w, err)
			return
		}
		write(w, http.StatusOK, b)
		return
	}
	action := parts[4]
	if action == "observations" && len(parts) >= 6 {
		s.correctObservation(w, r, id, parts)
		return
	}
	if r.Method == http.MethodGet {
		s.readAction(w, r, id, action)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var value any
	var err error
	switch action {
	case "identity":
		var request identityReq
		if err = decode(r, &request); err == nil {
			var structured []domain.IdentityEvidenceItem
			var legacy map[string]string
			structured, legacy, err = parseIdentityEvidence(request.Evidence)
			if err == nil {
				if legacy != nil {
					value, err = s.App.Identity(id, legacy, request.ConfirmedBy, requestRevision(r, request.ExpectedRevision))
				} else {
					value, err = s.App.IdentityStructured(id, structured, request.ConfirmedBy, requestRevision(r, request.ExpectedRevision))
				}
			}
		}
	case "protocol":
		var request struct {
			Protocol         domain.GerminationProtocol `json:"protocol"`
			ExpectedRevision *int                       `json:"expected_revision,omitempty"`
		}
		var direct domain.GerminationProtocol
		var expected *int
		direct, expected, err = decodeProtocol(r)
		if err == nil {
			request.Protocol, request.ExpectedRevision = direct, expected
			value, err = s.App.Protocol(id, request.Protocol, requestRevision(r, request.ExpectedRevision))
		}
	case "observations":
		var request observationReq
		if err = decode(r, &request); err == nil {
			observations := request.Observations
			if len(observations) == 0 {
				observations = []domain.TrialObservation{{AccessionID: request.AccessionID, ObservedBy: request.ObservedBy, DayIndex: request.DayIndex, GerminatedCount: request.GerminatedCount, DormantCount: request.DormantCount, MoldedCount: request.MoldedCount, DeadCount: request.DeadCount}}
			}
			for i := range observations {
				if observations[i].DayIndex == 0 {
					observations[i].DayIndex = request.DayIndex
				}
				if observations[i].ObservedBy == "" {
					observations[i].ObservedBy = request.ObservedBy
				}
			}
			value, err = s.App.ObserveBatch(id, observations, requestRevision(r, request.ExpectedRevision))
		}
	case "remediation":
		var request struct {
			domain.RemediationCase
			ExpectedRevision *int `json:"expected_revision,omitempty"`
		}
		err = decode(r, &request)
		if err == nil {
			value, err = s.App.Remediate(id, request.RemediationCase, requestRevision(r, request.ExpectedRevision))
		}
	case "retest":
		var request struct {
			CaseID           string `json:"case_id"`
			SampleSize       int    `json:"sample_size"`
			GerminatedCount  int    `json:"germinated_count"`
			DormantCount     int    `json:"dormant_count"`
			MoldedCount      int    `json:"molded_count"`
			DeadCount        int    `json:"dead_count"`
			Resolution       string `json:"resolution"`
			RequestID        string `json:"request_id"`
			ExpectedRevision *int   `json:"expected_revision,omitempty"`
		}
		err = decode(r, &request)
		if err == nil {
			value, err = s.App.RetestCounts(id, request.CaseID, domain.RetestResult{SampleSize: request.SampleSize, GerminatedCount: request.GerminatedCount, DormantCount: request.DormantCount, MoldedCount: request.MoldedCount, DeadCount: request.DeadCount}, request.Resolution, request.RequestID, requestRevision(r, request.ExpectedRevision))
		}
	case "review":
		var request struct {
			ReviewerID       string               `json:"reviewer_id"`
			Summary          string               `json:"summary"`
			Decision         string               `json:"decision,omitempty"`
			Approve          *bool                `json:"approve,omitempty"`
			Checks           []domain.ReviewCheck `json:"checks,omitempty"`
			ExpectedRevision *int                 `json:"expected_revision,omitempty"`
		}
		err = decode(r, &request)
		if err == nil {
			expected := requestRevision(r, request.ExpectedRevision)
			if len(request.Checks) == 0 && request.Approve != nil {
				value, err = s.App.Review(id, request.ReviewerID, request.Summary, *request.Approve, expected)
			} else {
				value, err = s.App.ReviewChecklist(id, request.ReviewerID, request.Summary, request.Decision, request.Checks, expected)
			}
		}
	case "archive":
		var request struct {
			ReleaseGrade     string `json:"release_grade"`
			RecheckDueDate   string `json:"recheck_due_date"`
			ExpectedRevision *int   `json:"expected_revision,omitempty"`
		}
		err = decode(r, &request)
		if err == nil {
			value, err = s.App.Archive(id, request.ReleaseGrade, request.RecheckDueDate, requestRevision(r, request.ExpectedRevision))
		}
	default:
		write(w, http.StatusNotFound, nil)
		return
	}
	if err != nil {
		errw(w, err)
		return
	}
	write(w, http.StatusOK, value)
}

func (s *Server) readAction(w http.ResponseWriter, r *http.Request, id, action string) {
	switch action {
	case "observations":
		b, err := s.App.Get(id)
		if err != nil {
			errw(w, err)
			return
		}
		observations := append([]domain.TrialObservation(nil), b.Observations...)
		query := r.URL.Query()
		if accession := query.Get("accession_id"); accession != "" {
			filtered := observations[:0]
			for _, observation := range observations {
				if observation.AccessionID == accession {
					filtered = append(filtered, observation)
				}
			}
			observations = filtered
		}
		if raw := query.Get("day_index"); raw != "" {
			day, parseErr := strconv.Atoi(raw)
			if parseErr != nil {
				errw(w, errors.New("day_index_invalid"))
				return
			}
			filtered := observations[:0]
			for _, observation := range observations {
				if observation.DayIndex == day {
					filtered = append(filtered, observation)
				}
			}
			observations = filtered
		}
		sort.Slice(observations, func(i, j int) bool {
			if observations[i].AccessionID == observations[j].AccessionID {
				return observations[i].DayIndex < observations[j].DayIndex
			}
			return observations[i].AccessionID < observations[j].AccessionID
		})
		page := len(observations)
		if raw := query.Get("page_size"); raw != "" {
			n, parseErr := strconv.Atoi(raw)
			if parseErr != nil || n < 1 {
				errw(w, errors.New("page_size_invalid"))
				return
			}
			page = n
		}
		start := 0
		if cursor := query.Get("cursor"); cursor != "" {
			for i, observation := range observations {
				if observation.ObservationID == cursor {
					start = i + 1
				}
			}
		}
		end := start + page
		if end > len(observations) {
			end = len(observations)
		}
		progress, completion, next := b.ObservationProgress()
		out := map[string]any{"items": observations[start:end], "metrics": map[string]any{"minimum_viability": minMetric(b), "completion_percent": completion, "next_observation_day": next, "progress": progress}}
		if end < len(observations) {
			out["next_cursor"] = observations[end-1].ObservationID
		}
		write(w, http.StatusOK, out)
	case "remediation":
		b, err := s.App.Get(id)
		if err != nil {
			errw(w, err)
			return
		}
		write(w, http.StatusOK, b.Remediations)
	case "events":
		if _, err := s.App.Get(id); err != nil {
			errw(w, err)
			return
		}
		write(w, http.StatusOK, s.App.Events(id))
	case "integrity":
		result, err := s.App.DiagnoseIntegrity(id)
		if err != nil {
			errw(w, err)
			return
		}
		write(w, http.StatusOK, result)
	case "evidence-package":
		result, err := s.App.EvidencePackage(id, r.URL.Query().Get("segment"))
		if err != nil {
			errw(w, err)
			return
		}
		write(w, http.StatusOK, result)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) correctObservation(w http.ResponseWriter, r *http.Request, id string, parts []string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if len(parts) != 6 && (len(parts) != 7 || parts[6] != "correction") {
		write(w, http.StatusNotFound, nil)
		return
	}
	var request correctionReq
	if err := decode(r, &request); err != nil {
		errw(w, err)
		return
	}
	b, err := s.App.CorrectObservation(id, parts[5], request.Correction, request.Reason, request.OperatorID, requestRevision(r, request.ExpectedRevision))
	if err != nil {
		errw(w, err)
		return
	}
	write(w, http.StatusOK, b)
}

func parseIdentityEvidence(raw json.RawMessage) ([]domain.IdentityEvidenceItem, map[string]string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil, errors.New("identity_evidence_required")
	}
	if trimmed[0] == '[' {
		var evidence []domain.IdentityEvidenceItem
		if err := decodeBytes(trimmed, &evidence); err != nil {
			return nil, nil, err
		}
		return evidence, nil, nil
	}
	var entries map[string]json.RawMessage
	if err := decodeBytes(trimmed, &entries); err != nil {
		return nil, nil, err
	}
	legacy := map[string]string{}
	structured := make([]domain.IdentityEvidenceItem, 0, len(entries))
	legacyOnly := true
	for accession, value := range entries {
		var text string
		if err := json.Unmarshal(value, &text); err == nil {
			legacy[accession] = text
			continue
		}
		legacyOnly = false
		var item domain.IdentityEvidenceItem
		if err := decodeBytes(value, &item); err != nil {
			return nil, nil, err
		}
		if item.AccessionID != "" && item.AccessionID != accession {
			return nil, nil, errors.New("evidence_accession_mismatch")
		}
		item.AccessionID = accession
		structured = append(structured, item)
	}
	if legacyOnly {
		return nil, legacy, nil
	}
	if len(legacy) > 0 {
		return nil, nil, errors.New("mixed_identity_evidence_format")
	}
	return structured, nil, nil
}

func decodeProtocol(r *http.Request) (domain.GerminationProtocol, *int, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return domain.GerminationProtocol{}, nil, err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return domain.GerminationProtocol{}, nil, err
	}
	var expected *int
	if value, ok := object["expected_revision"]; ok {
		var revision int
		if err := json.Unmarshal(value, &revision); err != nil {
			return domain.GerminationProtocol{}, nil, err
		}
		expected = &revision
		delete(object, "expected_revision")
	}
	if wrapped, ok := object["protocol"]; ok {
		if len(object) != 1 {
			return domain.GerminationProtocol{}, nil, errors.New("unknown_protocol_field")
		}
		var protocol domain.GerminationProtocol
		if err := decodeBytes(wrapped, &protocol); err != nil {
			return domain.GerminationProtocol{}, nil, err
		}
		return protocol, expected, nil
	}
	clean, _ := json.Marshal(object)
	var protocol domain.GerminationProtocol
	if err := decodeBytes(clean, &protocol); err != nil {
		return domain.GerminationProtocol{}, nil, err
	}
	return protocol, expected, nil
}
