-- Which figure the rider wants to see first (#616). Distance leading
-- everywhere was my choice, not theirs: for someone who mainly wants to know
-- whether they are getting faster, distance is the least interesting of the
-- three numbers on the page.
--
-- Nullable rather than defaulted in SQL: NULL means "never chosen", which the
-- application maps to the same order as before. A DEFAULT would make an
-- unchosen value indistinguishable from a deliberate one.
ALTER TABLE users ADD COLUMN primary_metric TEXT;
