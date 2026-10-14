-- Modify "device_authorizations" table
ALTER TABLE "device_authorizations" ADD COLUMN "status" character varying NOT NULL DEFAULT 'pending', ADD COLUMN "provider" character varying NULL, ADD COLUMN "state_hash" bytea NULL, ADD COLUMN "code_verifier" character varying NULL, ADD COLUMN "nonce" character varying NULL, ADD COLUMN "last_polled_at" timestamptz NULL;
-- Create index "device_authorizations_state_hash_key" to table: "device_authorizations"
CREATE UNIQUE INDEX "device_authorizations_state_hash_key" ON "device_authorizations" ("state_hash");
