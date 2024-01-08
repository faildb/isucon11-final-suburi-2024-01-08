-- Dropping tables in reverse order of creation
DROP TABLE IF EXISTS unread_announcements;
DROP TABLE IF EXISTS announcements;
DROP TABLE IF EXISTS submissions;
DROP TABLE IF EXISTS classes;
DROP TABLE IF EXISTS registrations;
DROP TABLE IF EXISTS courses;
DROP TABLE IF EXISTS users;

-- master data
-- 初手で外部キー制約を外す
CREATE TABLE users
(
    id              TEXT PRIMARY KEY,
    code            TEXT UNIQUE NOT NULL,
    name            TEXT NOT NULL,
    hashed_password BYTEA NOT NULL,
    type            TEXT CHECK (type IN ('student', 'teacher')) NOT NULL
);

CREATE TABLE courses
(
    id          TEXT PRIMARY KEY,
    code        TEXT UNIQUE NOT NULL,
    type        TEXT CHECK (type IN ('liberal-arts', 'major-subjects')) NOT NULL,
    name        TEXT NOT NULL,
    description TEXT NOT NULL,
    credit      SMALLINT NOT NULL,
    period      SMALLINT NOT NULL,
    day_of_week TEXT CHECK (day_of_week IN ('monday', 'tuesday', 'wednesday', 'thursday', 'friday')) NOT NULL,
    teacher_id  TEXT NOT NULL,
    keywords    TEXT NOT NULL,
    status      TEXT CHECK (status IN ('registration', 'in-progress', 'closed')) NOT NULL DEFAULT 'registration'
--    CONSTRAINT fk_courses_teacher_id FOREIGN KEY (teacher_id) REFERENCES users (id)
);

CREATE TABLE registrations
(
    course_id TEXT,
    user_id   TEXT,
    PRIMARY KEY (course_id, user_id)
--    CONSTRAINT fk_registrations_course_id FOREIGN KEY (course_id) REFERENCES courses (id),
--    CONSTRAINT fk_registrations_user_id FOREIGN KEY (user_id) REFERENCES users (id)
);

CREATE TABLE classes
(
    id                TEXT PRIMARY KEY,
    course_id         TEXT NOT NULL,
    part              SMALLINT NOT NULL,
    title             TEXT NOT NULL,
    description       TEXT NOT NULL,
    submission_closed BOOLEAN NOT NULL DEFAULT false,
    UNIQUE (course_id, part)
--    CONSTRAINT fk_classes_course_id FOREIGN KEY (course_id) REFERENCES courses (id)
);

CREATE TABLE submissions
(
    user_id   TEXT NOT NULL,
    class_id  TEXT NOT NULL,
    file_name TEXT NOT NULL,
    score     SMALLINT,
    PRIMARY KEY (user_id, class_id)
--    CONSTRAINT fk_submissions_user_id FOREIGN KEY (user_id) REFERENCES users (id),
--    CONSTRAINT fk_submissions_class_id FOREIGN KEY (class_id) REFERENCES classes (id)
);

CREATE TABLE announcements
(
    id        TEXT PRIMARY KEY,
    course_id TEXT NOT NULL,
    title     TEXT NOT NULL,
    message   TEXT NOT NULL
--    CONSTRAINT fk_announcements_course_id FOREIGN KEY (course_id) REFERENCES courses (id)
);

CREATE TABLE unread_announcements
(
    announcement_id TEXT NOT NULL,
    user_id         TEXT NOT NULL,
    is_deleted      BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (announcement_id, user_id)
--    CONSTRAINT fk_unread_announcements_announcement_id FOREIGN KEY (announcement_id) REFERENCES announcements (id),
--    CONSTRAINT fk_unread_announcements_user_id FOREIGN KEY (user_id) REFERENCES users (id)
);