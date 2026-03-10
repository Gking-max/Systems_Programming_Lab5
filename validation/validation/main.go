package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type envelope map[string]any

func (app *application) writeJSON(w http.ResponseWriter, status int, data envelope, headers http.Header) error {
	js, err := json.Marshal(data)
	if err != nil {
		return err
	}
	js = append(js, '\n')

	for key, values := range headers {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, err = w.Write(js)
	return err
}

func (app *application) readJSON(w http.ResponseWriter, r *http.Request, dst any) error {

	const maxBytes = 1_048_576
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	dec := json.NewDecoder(r.Body)

	dec.DisallowUnknownFields()

	err := dec.Decode(dst)
	if err != nil {

		var syntaxError *json.SyntaxError
		var unmarshalTypeError *json.UnmarshalTypeError
		var invalidUnmarshalError *json.InvalidUnmarshalError
		var maxBytesError *http.MaxBytesError

		switch {

		case errors.As(err, &syntaxError):
			return fmt.Errorf("body contains badly-formed JSON (at character %d)", syntaxError.Offset)

		case errors.Is(err, io.ErrUnexpectedEOF):
			return errors.New("body contains badly-formed JSON")

		case errors.As(err, &unmarshalTypeError):
			if unmarshalTypeError.Field != "" {
				return fmt.Errorf("body contains incorrect JSON type for field %q", unmarshalTypeError.Field)
			}
			return fmt.Errorf("body contains incorrect JSON type (at character %d)", unmarshalTypeError.Offset)

		case errors.Is(err, io.EOF):
			return errors.New("body must not be empty")

		case strings.Contains(err.Error(), "unknown field"):
			fieldName := strings.TrimPrefix(err.Error(), "json: unknown field ")
			return fmt.Errorf("body contains unknown key %s", fieldName)

		case errors.As(err, &maxBytesError):
			return fmt.Errorf("body must not be larger than %d bytes", maxBytesError.Limit)

		case errors.As(err, &invalidUnmarshalError):
			panic(err)

		default:
			return err
		}
	}

	err = dec.Decode(&struct{}{})
	if !errors.Is(err, io.EOF) {
		return errors.New("body must only contain a single JSON value")
	}

	return nil
}

// ─── Models ───────────────────────────────────────────────────────────────────

type Student struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Programme string `json:"programme"`
	Year      int    `json:"year"`
}

type Course struct {
	Code        string   `json:"code"`
	Title       string   `json:"title"`
	Credits     int      `json:"credits"`
	Enrolled    int      `json:"enrolled"`
	Instructors []string `json:"instructors"`
}

// ─── Application ──────────────────────────────────────────────────────────────

type application struct{}

// ─── Demo data ────────────────────────────────────────────────────────────────

var students = []Student{
	{1, "Eve Castillo", "BSc Computer Science", 2},
	{2, "Marco Tillett", "BSc Computer Science", 3},
	{3, "Aisha Gentle", "BSc Information Systems", 1},
	{4, "Raj Palacio", "BSc Computer Science", 4},
}

var courses = []Course{
	{
		Code:        "CMPS2212",
		Title:       "GUI Programming",
		Credits:     3,
		Enrolled:    28,
		Instructors: []string{"Boss"},
	},
	{
		Code:        "CMPS3412",
		Title:       "Database Systems",
		Credits:     3,
		Enrolled:    22,
		Instructors: []string{"Dr. Ramos"},
	},
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

// GET /students
// Returns all students.
func (app *application) listStudents(w http.ResponseWriter, r *http.Request) {
	err := app.writeJSON(w, http.StatusOK, envelope{"students": students}, nil)
	if err != nil {
		app.serverError(w, err)
	}
}

// GET /students/{id}
// Returns a single student.
// Demonstrates: path value parsing and validation, X-Resource-Id header.
func (app *application) getStudent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		app.notFound(w)
		return
	}

	for _, s := range students {
		if s.ID == id {
			extra := http.Header{
				"X-Resource-Id": []string{strconv.FormatInt(id, 10)},
			}
			err := app.writeJSON(w, http.StatusOK, envelope{"student": s}, extra)
			if err != nil {
				app.serverError(w, err)
			}
			return
		}
	}

	app.notFound(w)
}

// POST /students
// Creates a student (demo — nothing is persisted).
// Demonstrates: readJSON, Validator, 201 Created with Location header.
func (app *application) createStudent(w http.ResponseWriter, r *http.Request) {

	// input is a whitelist of the fields the client is permitted to send.
	// Fields on Student that the client must never set (e.g. ID) are
	// deliberately absent here.
	var input struct {
		Name      string `json:"name"`
		Programme string `json:"programme"`
		Year      int    `json:"year"`
	}

	// Step 1 — decode the request body into input.
	// readJSON handles structural problems (bad JSON, wrong types, unknown
	// fields). Any error here is the client's fault — respond 400.
	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, err.Error())
		return
	}

	// Step 2 — validate the decoded values against our business rules.
	// newValidator() initialises the Errors map so it is safe to write to.
	v := newValidator()

	// Name must be present and within the database column width.
	v.Check(input.Name != "",         "name", "must be provided")
	v.Check(len(input.Name) <= 100,   "name", "must not exceed 100 characters")

	// Programme must be present.
	v.Check(input.Programme != "",    "programme", "must be provided")

	// Year must be a valid academic year for this institution.
	v.Check(between(input.Year, 1, 4), "year", "must be between 1 and 4")

	// Step 3 — if any check failed, send all errors in one 422 response
	// and stop. Only valid data reaches the code below.
	if !v.Valid() {
		app.failedValidation(w, v.Errors)
		return
	}

	// Step 4 — business logic. All fields are known-good at this point.
	newStudent := Student{
		ID:        int64(len(students) + 1),
		Name:      input.Name,
		Programme: input.Programme,
		Year:      input.Year,
	}

	extra := http.Header{
		"Location": []string{"/students/" + strconv.FormatInt(newStudent.ID, 10)},
	}

	err = app.writeJSON(w, http.StatusCreated, envelope{"student": newStudent}, extra)
	if err != nil {
		app.serverError(w, err)
	}
}

// PUT /students/{id}
// Replaces a student record (demo — nothing is persisted).
// Demonstrates: readJSON on a PUT, Validator, 200 with updated resource.
func (app *application) updateStudent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		app.notFound(w)
		return
	}

	// input mirrors createStudent — the client cannot supply an ID.
	var input struct {
		Name      string `json:"name"`
		Programme string `json:"programme"`
		Year      int    `json:"year"`
	}

	// Step 1 — decode.
	err = app.readJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, err.Error())
		return
	}

	// Step 2 — validate. The rules are identical to createStudent.
	// When validation logic is shared across handlers, extract it into a
	// standalone function (e.g. validateStudent) to avoid duplication.
	v := newValidator()

	v.Check(input.Name != "",          "name", "must be provided")
	v.Check(len(input.Name) <= 100,    "name", "must not exceed 100 characters")

	v.Check(input.Programme != "",     "programme", "must be provided")

	v.Check(between(input.Year, 1, 4), "year", "must be between 1 and 4")

	// Step 3 — reject if any check failed.
	if !v.Valid() {
		app.failedValidation(w, v.Errors)
		return
	}

	// Step 4 — business logic.
	for _, s := range students {
		if s.ID == id {
			updated := Student{
				ID:        id,
				Name:      input.Name,
				Programme: input.Programme,
				Year:      input.Year,
			}
			err := app.writeJSON(w, http.StatusOK, envelope{"student": updated}, nil)
			if err != nil {
				app.serverError(w, err)
			}
			return
		}
	}

	app.notFound(w)
}

// GET /courses
// Returns all courses.
func (app *application) listCourses(w http.ResponseWriter, r *http.Request) {
	err := app.writeJSON(w, http.StatusOK, envelope{"courses": courses}, nil)
	if err != nil {
		app.serverError(w, err)
	}
}

// GET /health
// Health check.
// Demonstrates: Cache-Control as a caller-supplied header.
func (app *application) health(w http.ResponseWriter, r *http.Request) {
	extra := http.Header{
		"Cache-Control": []string{"public, max-age=30"},
	}
	err := app.writeJSON(w, http.StatusOK, envelope{
		"status":    "available",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}, extra)
	if err != nil {
		app.serverError(w, err)
	}
}

// GET /headers
// Echoes every request header back as JSON.
// Demonstrates: reading r.Header, computed response header.
func (app *application) echoHeaders(w http.ResponseWriter, r *http.Request) {
	received := make(map[string]string, len(r.Header))
	for name, values := range r.Header {
		received[name] = strings.Join(values, ", ")
	}

	extra := http.Header{
		"X-Total-Headers": []string{strconv.Itoa(len(received))},
	}

	err := app.writeJSON(w, http.StatusOK, envelope{
		"headers_received": received,
		"count":            len(received),
	}, extra)
	if err != nil {
		app.serverError(w, err)
	}
}

// ─── Error helpers ────────────────────────────────────────────────────────────

func (app *application) serverError(w http.ResponseWriter, err error) {
	log.Printf("ERROR: %v", err)
	app.writeJSON(w, http.StatusInternalServerError, envelope{
		"error": "the server encountered a problem and could not process your request",
	}, nil)
}

func (app *application) notFound(w http.ResponseWriter) {
	app.writeJSON(w, http.StatusNotFound, envelope{
		"error": "the requested resource could not be found",
	}, nil)
}

func (app *application) badRequest(w http.ResponseWriter, msg string) {
	app.writeJSON(w, http.StatusBadRequest, envelope{
		"error": msg,
	}, nil)
}

// failedValidation sends a 422 Unprocessable Entity response containing all
// field errors collected by the Validator.
//
// 400 Bad Request  — the JSON itself was malformed (readJSON errors).
// 422 Unprocessable — the JSON was valid but the data broke a business rule
//
// Keeping these two status codes distinct makes it easier for the frontend
// to decide how to handle the response: 400 means fix the request format,
// 422 means fix the field values.
func (app *application) failedValidation(w http.ResponseWriter, errors map[string]string) {
	app.writeJSON(w, http.StatusUnprocessableEntity, envelope{"errors": errors}, nil)
}

// ─── Routes & main ────────────────────────────────────────────────────────────

func main() {
	app := &application{}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /students",      app.listStudents)
	mux.HandleFunc("GET /students/{id}", app.getStudent)
	mux.HandleFunc("POST /students",     app.createStudent)
	mux.HandleFunc("PUT /students/{id}", app.updateStudent)
	mux.HandleFunc("GET /courses",       app.listCourses)
	mux.HandleFunc("GET /health",        app.health)
	mux.HandleFunc("GET /headers",       app.echoHeaders)

	log.Println("Starting server on :4000")
	log.Println()
	log.Println("  GET    /students")
	log.Println("  GET    /students/{id}")
	log.Println("  POST   /students        body: {\"name\":\"...\",\"programme\":\"...\",\"year\":1}")
	log.Println("  PUT    /students/{id}   body: {\"name\":\"...\",\"programme\":\"...\",\"year\":1}")
	log.Println("  GET    /courses")
	log.Println("  GET    /health")
	log.Println("  GET    /headers")

	err := http.ListenAndServe(":4000", mux)
	log.Fatal(err)
}
