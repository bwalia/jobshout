-- Remove the backfilled system roles; user_roles rows cascade.
DELETE FROM roles WHERE is_system = true;
