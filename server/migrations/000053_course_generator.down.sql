DROP TABLE IF EXISTS course_chapters;
DROP TABLE IF EXISTS course_runs;
DELETE FROM agents WHERE metadata->>'builtin' = 'course_generator';
