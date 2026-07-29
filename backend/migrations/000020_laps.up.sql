CREATE TABLE laps (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    activity_id BIGINT NOT NULL REFERENCES activities (id) ON DELETE CASCADE,
    lap_index INTEGER NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    elapsed_seconds INTEGER NOT NULL,
    distance_meters DOUBLE PRECISION,
    avg_power_watts DOUBLE PRECISION,
    max_power_watts DOUBLE PRECISION,
    avg_heart_rate DOUBLE PRECISION,
    max_heart_rate DOUBLE PRECISION,
    avg_speed_mps DOUBLE PRECISION,
    max_speed_mps DOUBLE PRECISION,
    UNIQUE (activity_id, lap_index)
);

CREATE INDEX laps_activity_idx ON laps (activity_id, lap_index);
