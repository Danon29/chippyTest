-- +goose Up
CREATE TABLE users (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	email TEXT UNIQUE NOT NULL
);

-- +goose Down
DROP TABLE users;
