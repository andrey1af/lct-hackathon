CREATE TABLE users (
 id uuid PRIMARY KEY, email text NOT NULL UNIQUE, password_hash text NOT NULL, name text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE companies (
 id uuid PRIMARY KEY, name text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE memberships (
 company_id uuid NOT NULL REFERENCES companies(id), user_id uuid NOT NULL REFERENCES users(id),
 role text NOT NULL CHECK (role IN ('employee','company_admin','platform_admin')),
 status text NOT NULL CHECK (status IN ('active','inactive')),
 created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(company_id,user_id)
);
CREATE TABLE auth_sessions (
 id uuid PRIMARY KEY, user_id uuid NOT NULL REFERENCES users(id), token_hash text NOT NULL UNIQUE,
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE invitations (
 id uuid PRIMARY KEY, company_id uuid NOT NULL REFERENCES companies(id), email text NOT NULL,
 role text NOT NULL CHECK (role IN ('employee','company_admin')), token_hash text NOT NULL UNIQUE,
 expires_at timestamptz NOT NULL, accepted_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE cases (
 id uuid PRIMARY KEY, company_id uuid REFERENCES companies(id), key text NOT NULL UNIQUE,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE case_versions (
 id uuid PRIMARY KEY, case_id uuid NOT NULL REFERENCES cases(id), version int NOT NULL CHECK (version > 0),
 mode text NOT NULL CHECK (mode IN ('assessment','practice')),
 title text NOT NULL, difficulty text NOT NULL CHECK (difficulty IN ('easy','medium','hard')),
 skill_ids jsonb NOT NULL, briefing text NOT NULL, private_config jsonb NOT NULL,
 rubric jsonb NOT NULL, max_turns int NOT NULL CHECK (max_turns > 0),
 content_hash text NOT NULL, published_at timestamptz NOT NULL DEFAULT now(), UNIQUE(case_id,version)
);
CREATE TABLE modules (
 id uuid PRIMARY KEY, key text NOT NULL UNIQUE, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE module_versions (
 id uuid PRIMARY KEY, module_id uuid NOT NULL REFERENCES modules(id), version int NOT NULL CHECK (version > 0),
 order_index int NOT NULL CHECK (order_index > 0), title text NOT NULL, skill_ids jsonb NOT NULL,
 assessment_case_version_id uuid NOT NULL REFERENCES case_versions(id), content_hash text NOT NULL,
 published_at timestamptz NOT NULL DEFAULT now(), UNIQUE(module_id,version), UNIQUE(order_index,version)
);
CREATE TABLE steps (
 id uuid PRIMARY KEY, module_version_id uuid NOT NULL REFERENCES module_versions(id),
 order_index int NOT NULL CHECK (order_index > 0), type text NOT NULL CHECK (type IN ('theory','choice')),
 content text NOT NULL, options jsonb NOT NULL DEFAULT '[]'::jsonb, answer_key text NOT NULL,
 explanation text NOT NULL, UNIQUE(module_version_id,order_index)
);
CREATE TABLE sessions (
 id uuid PRIMARY KEY, company_id uuid NOT NULL REFERENCES companies(id), user_id uuid NOT NULL REFERENCES users(id),
 case_version_id uuid NOT NULL REFERENCES case_versions(id), module_version_id uuid REFERENCES module_versions(id),
 mode text NOT NULL CHECK (mode IN ('assessment','practice')),
 status text NOT NULL CHECK (status IN ('active','finished','evaluated')),
 state_json jsonb NOT NULL DEFAULT '{}'::jsonb, version int NOT NULL DEFAULT 1 CHECK (version > 0),
 created_at timestamptz NOT NULL DEFAULT now(), finished_at timestamptz,
 CHECK ((mode = 'assessment') = (module_version_id IS NOT NULL))
);
CREATE INDEX sessions_user_company_idx ON sessions(company_id,user_id,created_at);
CREATE TABLE messages (
 id uuid PRIMARY KEY, session_id uuid NOT NULL REFERENCES sessions(id), client_message_id text,
 turn_number int NOT NULL CHECK (turn_number > 0), role text NOT NULL CHECK (role IN ('user','opponent')),
 text text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(session_id,client_message_id), UNIQUE(session_id,turn_number,role)
);
CREATE TABLE assessments (
 id uuid PRIMARY KEY, session_id uuid NOT NULL UNIQUE REFERENCES sessions(id),
 rubric_version int NOT NULL CHECK (rubric_version > 0), criteria_json jsonb NOT NULL,
 passed boolean NOT NULL, feedback_json jsonb NOT NULL,
 evaluation_status text NOT NULL CHECK (evaluation_status IN ('pending','evaluated','needs_retry')),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE learning_attempts (
 id uuid PRIMARY KEY, company_id uuid NOT NULL REFERENCES companies(id), user_id uuid NOT NULL REFERENCES users(id),
 step_id uuid NOT NULL REFERENCES steps(id), module_version_id uuid NOT NULL REFERENCES module_versions(id),
 client_attempt_id text NOT NULL, answer text NOT NULL, correct boolean NOT NULL, completed boolean NOT NULL,
 result_json jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(company_id,user_id,client_attempt_id)
);
CREATE INDEX learning_attempts_scope_idx ON learning_attempts(company_id,user_id,module_version_id,step_id);
CREATE TABLE module_progress (
 company_id uuid NOT NULL REFERENCES companies(id), user_id uuid NOT NULL REFERENCES users(id),
 module_version_id uuid NOT NULL REFERENCES module_versions(id),
 status text NOT NULL CHECK (status IN ('locked','available','in_progress','assessment_ready','needs_retry','passed')),
 passed_session_id uuid REFERENCES sessions(id), updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(company_id,user_id,module_version_id)
);
