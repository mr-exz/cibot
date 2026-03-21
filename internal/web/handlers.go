package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// handleTicketForm renders the ticket creation form.
func (s *Server) handleTicketForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	groups, err := s.db.ListGroups(ctx)
	if err != nil {
		http.Error(w, "failed to load groups", http.StatusInternalServerError)
		return
	}

	// Only approved groups
	var approved []groupOption
	for _, g := range groups {
		if g.Approved {
			approved = append(approved, groupOption{ID: g.ChatID, Title: g.Title})
		}
	}

	data := struct {
		Groups []groupOption
	}{
		Groups: approved,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := ticketTmpl.Execute(w, data); err != nil {
		log.Printf("❌ template error: %v", err)
	}
}

type groupOption struct {
	ID    int64
	Title string
}

// handleTicketSubmit processes the submitted ticket form.
func (s *Server) handleTicketSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	groupID, err := strconv.ParseInt(r.FormValue("group_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid group", http.StatusBadRequest)
		return
	}

	threadID, _ := strconv.Atoi(r.FormValue("thread_id")) // 0 if main group

	categoryID, err := strconv.ParseInt(r.FormValue("category_id"), 10, 64)
	if err != nil || categoryID == 0 {
		http.Error(w, "invalid category", http.StatusBadRequest)
		return
	}

	typeID, err := strconv.ParseInt(r.FormValue("type_id"), 10, 64)
	if err != nil || typeID == 0 {
		http.Error(w, "invalid request type", http.StatusBadRequest)
		return
	}

	priority, _ := strconv.Atoi(r.FormValue("priority"))
	if priority < 0 || priority > 4 {
		priority = 0
	}

	reporterName := strings.TrimSpace(r.FormValue("reporter_name"))
	if reporterName == "" {
		http.Error(w, "reporter name is required", http.StatusBadRequest)
		return
	}

	msgURL := strings.TrimSpace(r.FormValue("msg_url"))
	msgBody := strings.TrimSpace(r.FormValue("msg_body"))

	if msgBody == "" {
		http.Error(w, "message body is required", http.StatusBadRequest)
		return
	}

	// Load category
	cat, err := s.db.GetCategory(ctx, categoryID)
	if err != nil {
		http.Error(w, "category not found", http.StatusBadRequest)
		return
	}

	// Verify category is valid for the chosen context
	cats, err := s.db.ListCategoriesForContext(ctx, groupID, threadID)
	if err != nil {
		http.Error(w, "failed to validate category", http.StatusInternalServerError)
		return
	}
	valid := false
	for _, c := range cats {
		if c.ID == categoryID {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "category not valid for selected context", http.StatusBadRequest)
		return
	}

	// Load request type
	rt, err := s.db.GetRequestType(ctx, typeID)
	if err != nil {
		http.Error(w, "request type not found", http.StatusBadRequest)
		return
	}

	// Build issue
	now := time.Now()
	title := buildTitle(msgBody, now)

	link := msgURL
	description := buildDescription(msgBody, reporterName, cat.Name, rt.Name, link)

	// Get on-duty person
	onDutyResult, err := s.db.GetOnDutyPersonResult(ctx, categoryID, now)
	if err != nil {
		log.Printf("⚠️  failed to get on-duty person: %v", err)
		onDutyResult = nil
	}

	assignee := ""
	if onDutyResult != nil && onDutyResult.Person != nil {
		assignee = onDutyResult.Person.LinearUsername
	}

	if onDutyResult != nil && !onDutyResult.Online {
		description += "\n\n⚠️ **Note:** Assigned person is currently outside working hours."
	}

	issueURL, err := s.linear.CreateIssue(ctx, title, description, cat.LinearTeamKey, assignee, []string{cat.Name, rt.Name}, priority)
	if err != nil {
		log.Printf("❌ failed to create Linear issue: %v", err)
		http.Error(w, fmt.Sprintf("failed to create issue: %v", err), http.StatusInternalServerError)
		return
	}

	assignedTo := "unassigned"
	if onDutyResult != nil && onDutyResult.Person != nil {
		assignedTo = onDutyResult.Person.Name
	}
	log.Printf("✓ Web ticket created: %s (assigned to %s, reporter: %s)", issueURL, assignedTo, reporterName)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, successHTML, cat.Name, assignedTo, issueURL, issueURL)
}

func buildTitle(body string, t time.Time) string {
	words := strings.Fields(body)
	if len(words) > 5 {
		words = words[:5]
	}
	snippet := strings.Join(words, " ")
	if len(strings.Fields(body)) > 5 {
		snippet += "..."
	}
	return fmt.Sprintf("%s (%s)", snippet, t.Format("2006-01-02 15:04"))
}

func buildDescription(msgBody, reporterName, categoryName, typeName, link string) string {
	desc := ""
	if msgBody != "" {
		desc = fmt.Sprintf("**💬 Message**\n%s\n\n", msgBody)
	}
	desc += fmt.Sprintf("**📌 Web Source**\n"+
		"- **Reporter:** %s\n"+
		"- **Category:** %s\n"+
		"- **Type:** %s",
		reporterName,
		categoryName,
		typeName,
	)
	if link != "" {
		desc += fmt.Sprintf("\n- **Link:** %s", link)
	}
	return desc
}

// handleAPITopics returns topics for a group as JSON.
func (s *Server) handleAPITopics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chatIDStr := r.URL.Query().Get("group")
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		http.Error(w, "bad group", http.StatusBadRequest)
		return
	}

	topics, err := s.db.LoadTopicsForChat(ctx, chatID)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	type topicItem struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	var items []topicItem
	for id, name := range topics {
		items = append(items, topicItem{ID: id, Name: name})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items) //nolint:errcheck
}

// handleAPICategories returns categories for a group+topic context as JSON.
func (s *Server) handleAPICategories(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chatID, err := strconv.ParseInt(r.URL.Query().Get("group"), 10, 64)
	if err != nil {
		http.Error(w, "bad group", http.StatusBadRequest)
		return
	}
	threadID, _ := strconv.Atoi(r.URL.Query().Get("topic"))

	cats, err := s.db.ListCategoriesForContext(ctx, chatID, threadID)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	type catItem struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Emoji string `json:"emoji"`
	}
	items := make([]catItem, len(cats))
	for i, c := range cats {
		items[i] = catItem{ID: c.ID, Name: c.Name, Emoji: c.Emoji}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items) //nolint:errcheck
}

// handleAPITypes returns request types for a category as JSON.
func (s *Server) handleAPITypes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	catID, err := strconv.ParseInt(r.URL.Query().Get("category"), 10, 64)
	if err != nil {
		http.Error(w, "bad category", http.StatusBadRequest)
		return
	}

	types, err := s.db.ListRequestTypesForCategory(ctx, catID)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	type typeItem struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	items := make([]typeItem, len(types))
	for i, t := range types {
		items[i] = typeItem{ID: t.ID, Name: t.Name}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items) //nolint:errcheck
}

const successHTML = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><title>Ticket Created</title>
<style>
body{font-family:system-ui,sans-serif;max-width:520px;margin:80px auto;padding:0 20px;color:#1a1a2e}
.card{background:#f0fdf4;border:1px solid #86efac;border-radius:12px;padding:32px}
h2{margin:0 0 16px;color:#166534}
p{margin:6px 0;color:#374151}
a{color:#1d4ed8}
.back{display:inline-block;margin-top:24px;color:#6b7280;text-decoration:none;font-size:14px}
.back:hover{color:#111827}
</style>
</head>
<body>
<div class="card">
  <h2>Ticket created</h2>
  <p><strong>Category:</strong> %s</p>
  <p><strong>Assigned to:</strong> %s</p>
  <p><strong>Linear:</strong> <a href="%s" target="_blank">%s</a></p>
</div>
<a class="back" href="/ticket">Submit another ticket</a>
</body>
</html>`
