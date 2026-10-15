-- Modify "task_events" table
ALTER TABLE "task_events" ADD COLUMN "change_id" uuid NOT NULL;
-- Create index "taskevent_change_id" to table: "task_events"
CREATE INDEX "taskevent_change_id" ON "task_events" ("change_id");
