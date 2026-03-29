// =============================================================================
// Package handlers — booking endpoints
// Location: golden-app/internal/handlers/bookings.go
//
// HTTP handlers for booking-related endpoints:
//   POST /bookings         — create a new booking
//   GET  /bookings/{ref}   — get a booking by reference number
// =============================================================================

package handlers

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/platform-eng/golden-app/internal/db"
)

// Booking represents a completed booking as returned by the API.
type Booking struct {
	ID               int       `json:"id"`
	BookingReference string    `json:"booking_reference"`
	PassengerName    string    `json:"passenger_name"`
	PassengerEmail   string    `json:"passenger_email"`
	FlightNumber     string    `json:"flight_number"`
	SeatNumber       string    `json:"seat_number"`
	Class            string    `json:"class"`
	Status           string    `json:"status"`
	BookedAt         time.Time `json:"booked_at"`
}

// CreateBooking handles POST /bookings
// Books a seat on a flight for a passenger.
//
// The booking flow:
//   1. Find or create the passenger
//   2. Verify the seat exists and is available
//   3. Mark the seat as unavailable
//   4. Create the booking record
//   5. Return the booking reference
//
// All steps run in a single transaction so if anything fails,
// the seat remains available and no partial booking is created.
func CreateBooking(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FirstName      string `json:"first_name"`
		LastName       string `json:"last_name"`
		Email          string `json:"email"`
		PassportNumber string `json:"passport_number"`
		SeatID         int    `json:"seat_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Email == "" || req.FirstName == "" || req.LastName == "" || req.SeatID == 0 {
		respondError(w, http.StatusBadRequest,
			"first_name, last_name, email, and seat_id are required")
		return
	}

	// Begin a transaction — all steps must succeed or all are rolled back.
	// This is the key safety guarantee: a passenger cannot be charged
	// and their seat not saved, or a seat marked unavailable with no booking.
	tx, err := db.DB.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "error starting transaction")
		return
	}
	defer tx.Rollback(r.Context())

	// Step 1 — find or create the passenger
	// INSERT ... ON CONFLICT ... DO UPDATE is an "upsert".
	// If the email already exists, we update the name and return the existing ID.
	// RETURNING id gives us the passenger ID without a separate SELECT.
	var passengerID int
	err = tx.QueryRow(r.Context(), `
		INSERT INTO passengers (first_name, last_name, email, passport_number)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (email) DO UPDATE
			SET first_name = EXCLUDED.first_name,
			    last_name  = EXCLUDED.last_name
		RETURNING id
	`, req.FirstName, req.LastName, req.Email, req.PassportNumber).Scan(&passengerID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "error creating passenger")
		return
	}

	// Step 2 — verify the seat is available
	// FOR UPDATE locks the row so no other concurrent request can book
	// the same seat between our check and the update below.
	// This prevents double-booking under concurrent load.
	var available bool
	err = tx.QueryRow(r.Context(), `
		SELECT available FROM seats WHERE id = $1 FOR UPDATE
	`, req.SeatID).Scan(&available)
	if err != nil {
		respondError(w, http.StatusNotFound, "seat not found")
		return
	}

	if !available {
		respondError(w, http.StatusConflict, "seat is no longer available")
		return
	}

	// Step 3 — mark the seat as unavailable
	_, err = tx.Exec(r.Context(), `
		UPDATE seats SET available = FALSE WHERE id = $1
	`, req.SeatID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "error reserving seat")
		return
	}

	// Step 4 — generate a unique booking reference and create the booking
	// Booking references are 6 uppercase alphanumeric characters (e.g. PE7X2K)
	ref := generateBookingReference()

	var bookingID int
	err = tx.QueryRow(r.Context(), `
		INSERT INTO bookings (passenger_id, seat_id, booking_reference)
		VALUES ($1, $2, $3)
		RETURNING id
	`, passengerID, req.SeatID, ref).Scan(&bookingID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "error creating booking")
		return
	}

	// Step 5 — commit the transaction
	if err := tx.Commit(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, "error completing booking")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"booking_id":        bookingID,
		"booking_reference": ref,
		"message":           "booking confirmed",
	})
}

// GetBooking handles GET /bookings/{ref}
// Returns full booking details by booking reference number.
func GetBooking(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "ref")
	if ref == "" {
		respondError(w, http.StatusBadRequest, "booking reference is required")
		return
	}

	query := `
		SELECT
			b.id,
			b.booking_reference,
			p.first_name || ' ' || p.last_name AS passenger_name,
			p.email,
			f.flight_number,
			s.seat_number,
			s.class,
			b.status,
			b.booked_at
		FROM bookings b
		JOIN passengers p ON b.passenger_id = p.id
		JOIN seats      s ON b.seat_id = s.id
		JOIN flights    f ON s.flight_id = f.id
		WHERE b.booking_reference = $1
	`

	var booking Booking
	err := db.DB.QueryRow(r.Context(), query, ref).Scan(
		&booking.ID,
		&booking.BookingReference,
		&booking.PassengerName,
		&booking.PassengerEmail,
		&booking.FlightNumber,
		&booking.SeatNumber,
		&booking.Class,
		&booking.Status,
		&booking.BookedAt,
	)
	if err != nil {
		respondError(w, http.StatusNotFound, "booking not found")
		return
	}

	respondJSON(w, http.StatusOK, booking)
}

// generateBookingReference creates a random 6-character uppercase
// alphanumeric booking reference (e.g. PE7X2K, A3BF9Q).
// In production this would check for uniqueness against the database.
func generateBookingReference() string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	var sb strings.Builder
	for i := 0; i < 6; i++ {
		sb.WriteByte(chars[rng.Intn(len(chars))])
	}
	return fmt.Sprintf("PE%s", sb.String()[:4])
}
