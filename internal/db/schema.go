// =============================================================================
// Package db — schema and seed data
// Location: golden-app/internal/db/schema.go
//
// This file contains:
//   - The SQL to create all tables (CreateSchema)
//   - The SQL to insert realistic seed data (SeedData)
//
// Both functions are idempotent — safe to call multiple times.
// CREATE TABLE IF NOT EXISTS and INSERT ... ON CONFLICT DO NOTHING
// ensure re-running does not fail or duplicate data.
// =============================================================================

package db

import (
	"context"
	"fmt"
)

// CreateSchema creates all tables if they do not already exist.
// Called once at server startup before accepting requests.
//
// Database schema:
//
//	airports  — IATA codes, city, country
//	flights   — scheduled flights between airports
//	seats     — individual seats on each flight
//	passengers — people who make bookings
//	bookings  — a passenger booking a specific seat on a flight
func CreateSchema(ctx context.Context) error {
	// We use a single transaction for all table creation.
	// If any table fails to create, the transaction rolls back
	// and no partial schema is left behind.
	tx, err := DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	// defer ensures Rollback is called if we return early due to an error.
	// If Commit() is called first, Rollback() becomes a no-op.
	defer tx.Rollback(ctx)

	schema := `
		-- airports: the origin and destination for flights
		-- IATA codes are the standard 3-letter airport identifiers (LAX, JFK, LHR)
		CREATE TABLE IF NOT EXISTS airports (
			id          SERIAL PRIMARY KEY,
			iata_code   VARCHAR(3)   NOT NULL UNIQUE,
			name        VARCHAR(255) NOT NULL,
			city        VARCHAR(255) NOT NULL,
			country     VARCHAR(255) NOT NULL,
			created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
		);

		-- flights: scheduled service between two airports
		CREATE TABLE IF NOT EXISTS flights (
			id               SERIAL PRIMARY KEY,
			flight_number    VARCHAR(10)  NOT NULL UNIQUE,
			origin_id        INTEGER      NOT NULL REFERENCES airports(id),
			destination_id   INTEGER      NOT NULL REFERENCES airports(id),
			departure_time   TIMESTAMPTZ  NOT NULL,
			arrival_time     TIMESTAMPTZ  NOT NULL,
			aircraft_type    VARCHAR(50)  NOT NULL,
			status           VARCHAR(20)  NOT NULL DEFAULT 'scheduled',
			created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

			-- A flight cannot depart before it arrives (sanity check)
			CONSTRAINT valid_times CHECK (arrival_time > departure_time),
			-- Status must be one of these values
			CONSTRAINT valid_status CHECK (status IN ('scheduled','boarding','departed','arrived','cancelled'))
		);

		-- seats: individual seats on a flight
		-- Each seat belongs to exactly one flight
		CREATE TABLE IF NOT EXISTS seats (
			id            SERIAL PRIMARY KEY,
			flight_id     INTEGER      NOT NULL REFERENCES flights(id),
			seat_number   VARCHAR(5)   NOT NULL,
			class         VARCHAR(20)  NOT NULL DEFAULT 'economy',
			available     BOOLEAN      NOT NULL DEFAULT TRUE,
			created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

			-- A seat number must be unique per flight
			UNIQUE(flight_id, seat_number),
			CONSTRAINT valid_class CHECK (class IN ('economy','business','first'))
		);

		-- passengers: people who make bookings
		CREATE TABLE IF NOT EXISTS passengers (
			id              SERIAL PRIMARY KEY,
			first_name      VARCHAR(255) NOT NULL,
			last_name       VARCHAR(255) NOT NULL,
			email           VARCHAR(255) NOT NULL UNIQUE,
			passport_number VARCHAR(50),
			created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
		);

		-- bookings: connects a passenger to a seat on a flight
		-- This is the core transaction in the airline domain
		CREATE TABLE IF NOT EXISTS bookings (
			id                SERIAL PRIMARY KEY,
			passenger_id      INTEGER      NOT NULL REFERENCES passengers(id),
			seat_id           INTEGER      NOT NULL REFERENCES seats(id),
			booking_reference VARCHAR(10)  NOT NULL UNIQUE,
			status            VARCHAR(20)  NOT NULL DEFAULT 'confirmed',
			booked_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

			-- A seat can only have one active booking
			UNIQUE(seat_id),
			CONSTRAINT valid_booking_status CHECK (status IN ('confirmed','cancelled','checked_in'))
		);

		-- Index on flight_number for fast lookup
		CREATE INDEX IF NOT EXISTS idx_flights_number ON flights(flight_number);
		-- Index on bookings reference for fast lookup
		CREATE INDEX IF NOT EXISTS idx_bookings_reference ON bookings(booking_reference);
		-- Index on passenger email for fast lookup
		CREATE INDEX IF NOT EXISTS idx_passengers_email ON passengers(email);
	`

	if _, err := tx.Exec(ctx, schema); err != nil {
		return fmt.Errorf("creating schema: %w", err)
	}

	return tx.Commit(ctx)
}

// SeedData inserts realistic initial data into the database.
// Uses ON CONFLICT DO NOTHING so re-running is safe — no duplicates.
// Called once at startup after CreateSchema.
func SeedData(ctx context.Context) error {
	tx, err := DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Insert airports
	// ON CONFLICT (iata_code) DO NOTHING means if the airport already
	// exists (from a previous startup), skip it silently.
	airports := `
		INSERT INTO airports (iata_code, name, city, country) VALUES
			('LAX', 'Los Angeles International', 'Los Angeles', 'USA'),
			('JFK', 'John F. Kennedy International', 'New York', 'USA'),
			('ORD', 'O Hare International', 'Chicago', 'USA'),
			('SFO', 'San Francisco International', 'San Francisco', 'USA'),
			('MIA', 'Miami International', 'Miami', 'USA'),
			('SEA', 'Seattle-Tacoma International', 'Seattle', 'USA'),
			('DFW', 'Dallas Fort Worth International', 'Dallas', 'USA'),
			('BOS', 'Boston Logan International', 'Boston', 'USA')
		ON CONFLICT (iata_code) DO NOTHING;
	`
	if _, err := tx.Exec(ctx, airports); err != nil {
		return fmt.Errorf("seeding airports: %w", err)
	}

	// Insert flights using airport IDs via subquery
	// The subquery (SELECT id FROM airports WHERE iata_code = 'LAX')
	// avoids hardcoding IDs which could differ between environments
	flights := `
		INSERT INTO flights (flight_number, origin_id, destination_id, departure_time, arrival_time, aircraft_type, status)
		VALUES
			('PE101',
				(SELECT id FROM airports WHERE iata_code = 'LAX'),
				(SELECT id FROM airports WHERE iata_code = 'JFK'),
				NOW() + INTERVAL '2 hours',
				NOW() + INTERVAL '7 hours 30 minutes',
				'Boeing 737', 'scheduled'),
			('PE102',
				(SELECT id FROM airports WHERE iata_code = 'JFK'),
				(SELECT id FROM airports WHERE iata_code = 'LAX'),
				NOW() + INTERVAL '4 hours',
				NOW() + INTERVAL '9 hours 30 minutes',
				'Boeing 737', 'scheduled'),
			('PE201',
				(SELECT id FROM airports WHERE iata_code = 'SFO'),
				(SELECT id FROM airports WHERE iata_code = 'ORD'),
				NOW() + INTERVAL '1 hour',
				NOW() + INTERVAL '5 hours',
				'Airbus A320', 'boarding'),
			('PE301',
				(SELECT id FROM airports WHERE iata_code = 'MIA'),
				(SELECT id FROM airports WHERE iata_code = 'BOS'),
				NOW() + INTERVAL '3 hours',
				NOW() + INTERVAL '5 hours 30 minutes',
				'Airbus A319', 'scheduled'),
			('PE401',
				(SELECT id FROM airports WHERE iata_code = 'SEA'),
				(SELECT id FROM airports WHERE iata_code = 'DFW'),
				NOW() + INTERVAL '6 hours',
				NOW() + INTERVAL '10 hours',
				'Boeing 757', 'scheduled')
		ON CONFLICT (flight_number) DO NOTHING;
	`
	if _, err := tx.Exec(ctx, flights); err != nil {
		return fmt.Errorf("seeding flights: %w", err)
	}

	// Insert seats for each flight
	// generate_series(1, 30) generates numbers 1 through 30
	// We create 30 seats per flight: rows 1-5 business, 6-30 economy
	seats := `
		INSERT INTO seats (flight_id, seat_number, class, available)
		SELECT
			f.id,
			row_num || seat_letter AS seat_number,
			CASE WHEN row_num <= 5 THEN 'business' ELSE 'economy' END AS class,
			TRUE AS available
		FROM
			flights f,
			generate_series(1, 30) AS row_num,
			unnest(ARRAY['A','B','C','D','E','F']) AS seat_letter
		ON CONFLICT (flight_id, seat_number) DO NOTHING;
	`
	if _, err := tx.Exec(ctx, seats); err != nil {
		return fmt.Errorf("seeding seats: %w", err)
	}

	// Insert a sample passenger
	passengers := `
		INSERT INTO passengers (first_name, last_name, email, passport_number)
		VALUES
			('Faisal', 'Afzal', 'faisal@platform-eng.dev', 'P12345678'),
			('Jane', 'Smith', 'jane.smith@example.com', 'P87654321')
		ON CONFLICT (email) DO NOTHING;
	`
	if _, err := tx.Exec(ctx, passengers); err != nil {
		return fmt.Errorf("seeding passengers: %w", err)
	}

	return tx.Commit(ctx)
}
