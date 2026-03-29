// =============================================================================
// Package handlers — flight endpoints
// Location: golden-app/internal/handlers/flights.go
//
// HTTP handlers for flight-related endpoints:
//   GET  /flights              — list all flights
//   GET  /flights/{id}         — get one flight by ID
//   POST /flights              — create a new flight
//   GET  /flights/{id}/seats   — list available seats on a flight
// =============================================================================

package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/platform-eng/golden-app/internal/db"
)

// Flight represents a flight as returned by the API.
// The json tags control how fields are serialised to JSON.
// omitempty means the field is omitted from JSON if it is the zero value.
type Flight struct {
	ID            int       `json:"id"`
	FlightNumber  string    `json:"flight_number"`
	Origin        Airport   `json:"origin"`
	Destination   Airport   `json:"destination"`
	DepartureTime time.Time `json:"departure_time"`
	ArrivalTime   time.Time `json:"arrival_time"`
	AircraftType  string    `json:"aircraft_type"`
	Status        string    `json:"status"`
}

// Airport is embedded inside Flight for origin and destination.
type Airport struct {
	ID       int    `json:"id"`
	IATACode string `json:"iata_code"`
	Name     string `json:"name"`
	City     string `json:"city"`
	Country  string `json:"country"`
}

// Seat represents an individual seat on a flight.
type Seat struct {
	ID         int    `json:"id"`
	SeatNumber string `json:"seat_number"`
	Class      string `json:"class"`
	Available  bool   `json:"available"`
}

// ListFlights handles GET /flights
// Returns all flights with their origin and destination airport details.
func ListFlights(w http.ResponseWriter, r *http.Request) {
	// The query joins flights with airports twice — once for origin,
	// once for destination. We alias the airports table as 'o' and 'd'
	// to distinguish between the two joins.
	query := `
		SELECT
			f.id, f.flight_number,
			o.id, o.iata_code, o.name, o.city, o.country,
			d.id, d.iata_code, d.name, d.city, d.country,
			f.departure_time, f.arrival_time,
			f.aircraft_type, f.status
		FROM flights f
		JOIN airports o ON f.origin_id = o.id
		JOIN airports d ON f.destination_id = d.id
		ORDER BY f.departure_time ASC
	`

	// r.Context() carries the request context including cancellation.
	// If the client disconnects, the context is cancelled and the
	// query is automatically terminated.
	rows, err := db.DB.Query(r.Context(), query)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "error querying flights")
		return
	}
	// rows.Close() must be called to release the connection back to the pool.
	// defer ensures it runs even if we return early.
	defer rows.Close()

	var flights []Flight
	for rows.Next() {
		var f Flight
		err := rows.Scan(
			&f.ID, &f.FlightNumber,
			&f.Origin.ID, &f.Origin.IATACode, &f.Origin.Name,
			&f.Origin.City, &f.Origin.Country,
			&f.Destination.ID, &f.Destination.IATACode, &f.Destination.Name,
			&f.Destination.City, &f.Destination.Country,
			&f.DepartureTime, &f.ArrivalTime,
			&f.AircraftType, &f.Status,
		)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "error scanning flight")
			return
		}
		flights = append(flights, f)
	}

	// rows.Err() returns any error that occurred during iteration.
	// This catches network errors that happen mid-stream.
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "error reading flights")
		return
	}

	// Return empty array not null if no flights exist
	if flights == nil {
		flights = []Flight{}
	}

	respondJSON(w, http.StatusOK, flights)
}

// GetFlight handles GET /flights/{id}
// Returns a single flight by its database ID.
func GetFlight(w http.ResponseWriter, r *http.Request) {
	// chi.URLParam extracts URL parameters defined in the route.
	// The route is registered as GET /flights/{id} so {id} is the param.
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid flight ID — must be an integer")
		return
	}

	query := `
		SELECT
			f.id, f.flight_number,
			o.id, o.iata_code, o.name, o.city, o.country,
			d.id, d.iata_code, d.name, d.city, d.country,
			f.departure_time, f.arrival_time,
			f.aircraft_type, f.status
		FROM flights f
		JOIN airports o ON f.origin_id = o.id
		JOIN airports d ON f.destination_id = d.id
		WHERE f.id = $1
	`

	// $1 is a positional parameter — pgx substitutes the value safely.
	// Never use string formatting to build SQL queries — that leads to
	// SQL injection vulnerabilities. Always use $1, $2, etc.
	var f Flight
	err = db.DB.QueryRow(r.Context(), query, id).Scan(
		&f.ID, &f.FlightNumber,
		&f.Origin.ID, &f.Origin.IATACode, &f.Origin.Name,
		&f.Origin.City, &f.Origin.Country,
		&f.Destination.ID, &f.Destination.IATACode, &f.Destination.Name,
		&f.Destination.City, &f.Destination.Country,
		&f.DepartureTime, &f.ArrivalTime,
		&f.AircraftType, &f.Status,
	)
	if err != nil {
		respondError(w, http.StatusNotFound, "flight not found")
		return
	}

	respondJSON(w, http.StatusOK, f)
}

// CreateFlight handles POST /flights
// Creates a new flight. Expects JSON body with flight details.
func CreateFlight(w http.ResponseWriter, r *http.Request) {
	// Request body struct — only the fields we accept from the client.
	// We do not accept id, created_at etc. — those are set by the database.
	var req struct {
		FlightNumber  string    `json:"flight_number"`
		OriginCode    string    `json:"origin_iata_code"`
		DestCode      string    `json:"destination_iata_code"`
		DepartureTime time.Time `json:"departure_time"`
		ArrivalTime   time.Time `json:"arrival_time"`
		AircraftType  string    `json:"aircraft_type"`
	}

	// json.NewDecoder reads the request body as a stream and decodes JSON.
	// This is more memory-efficient than reading the whole body first.
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Basic validation
	if req.FlightNumber == "" || req.OriginCode == "" || req.DestCode == "" {
		respondError(w, http.StatusBadRequest, "flight_number, origin_iata_code, and destination_iata_code are required")
		return
	}

	if req.ArrivalTime.Before(req.DepartureTime) {
		respondError(w, http.StatusBadRequest, "arrival_time must be after departure_time")
		return
	}

	// Insert the flight, looking up airport IDs by IATA code
	var flightID int
	err := db.DB.QueryRow(r.Context(), `
		INSERT INTO flights (flight_number, origin_id, destination_id, departure_time, arrival_time, aircraft_type)
		VALUES (
			$1,
			(SELECT id FROM airports WHERE iata_code = $2),
			(SELECT id FROM airports WHERE iata_code = $3),
			$4, $5, $6
		)
		RETURNING id
	`, req.FlightNumber, req.OriginCode, req.DestCode,
		req.DepartureTime, req.ArrivalTime, req.AircraftType).Scan(&flightID)

	if err != nil {
		respondError(w, http.StatusInternalServerError, "error creating flight: "+err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"id":      flightID,
		"message": "flight created successfully",
	})
}

// ListSeats handles GET /flights/{id}/seats
// Returns all seats for a flight, optionally filtering by availability.
func ListSeats(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	flightID, err := strconv.Atoi(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid flight ID")
		return
	}

	// Optional query parameter: ?available=true filters to available seats only
	// r.URL.Query() parses the query string into a map
	availableOnly := r.URL.Query().Get("available") == "true"

	query := `
		SELECT id, seat_number, class, available
		FROM seats
		WHERE flight_id = $1
	`
	args := []interface{}{flightID}

	// Dynamically add the available filter if requested
	if availableOnly {
		query += " AND available = TRUE"
	}

	query += " ORDER BY seat_number ASC"

	rows, err := db.DB.Query(r.Context(), query, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "error querying seats")
		return
	}
	defer rows.Close()

	var seats []Seat
	for rows.Next() {
		var s Seat
		if err := rows.Scan(&s.ID, &s.SeatNumber, &s.Class, &s.Available); err != nil {
			respondError(w, http.StatusInternalServerError, "error scanning seat")
			return
		}
		seats = append(seats, s)
	}

	if seats == nil {
		seats = []Seat{}
	}

	respondJSON(w, http.StatusOK, seats)
}

// =============================================================================
// Shared response helpers
// =============================================================================

// respondJSON writes a JSON response with the given status code.
// All handlers use this to ensure consistent response format and headers.
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondError writes a JSON error response.
// All error responses have the same shape: {"error": "message"}
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}
