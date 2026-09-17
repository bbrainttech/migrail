ALTER TABLE "orders" ADD COLUMN "status" text NOT NULL;--> statement-breakpoint
CREATE INDEX "orders_status_idx" ON "orders" USING btree ("status");
