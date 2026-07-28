CREATE TABLE power_curve_points (
    activity_id BIGINT NOT NULL REFERENCES activities (id) ON DELETE CASCADE,
    duration_seconds INTEGER NOT NULL,
    watts INTEGER NOT NULL,
    PRIMARY KEY (activity_id, duration_seconds)
);
