CREATE TABLE samples (
    activity_id BIGINT NOT NULL REFERENCES activities (id) ON DELETE CASCADE,
    time TIMESTAMPTZ NOT NULL,
    lat DOUBLE PRECISION,
    lon DOUBLE PRECISION,
    altitude_meters DOUBLE PRECISION,
    power_watts INTEGER,
    heart_rate INTEGER,
    cadence INTEGER,
    speed_mps DOUBLE PRECISION,
    temperature_celsius DOUBLE PRECISION,
    PRIMARY KEY (activity_id, time)
);

SELECT create_hypertable('samples', 'time');

CREATE INDEX samples_activity_time_idx ON samples (activity_id, time);
