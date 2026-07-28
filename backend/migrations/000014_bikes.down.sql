DROP INDEX IF EXISTS activities_bike_idx;
ALTER TABLE activities DROP COLUMN bike_id;
ALTER TABLE users DROP COLUMN active_bike_id;
DROP TABLE bikes;
