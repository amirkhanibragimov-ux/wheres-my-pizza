-- Wrap everything in a transaction to prevent partial table setup.
BEGIN;


-- Clean reset to ensure the clean setup.
DROP TABLE IF EXISTS order_status_log CASCADE;
DROP TABLE IF EXISTS order_items CASCADE;
DROP TABLE IF EXISTS orders CASCADE;
DROP TABLE IF EXISTS order_number_seq CASCADE;
DROP TABLE IF EXISTS workers CASCADE;
DROP TYPE  IF EXISTS order_status;
DROP TYPE  IF EXISTS order_type;
DROP TYPE  IF EXISTS worker_status;


-- =============================
-- Enumerations
-- =============================

CREATE TYPE order_status AS ENUM ('received', 'cooking', 'ready', 'completed', 'cancelled');
CREATE TYPE order_type AS ENUM ('dine_in', 'takeout', 'delivery');
CREATE TYPE worker_status AS ENUM ('online', 'offline');


-- =============================
-- Tables
-- =============================

-- Create table for storing registry for all kitchen workers, their specializations, and their current status.
CREATE TABLE workers (
    id                SERIAL        PRIMARY KEY,
    created_at        TIMESTAMPTZ   NOT NULL    DEFAULT NOW(),
    name              TEXT          UNIQUE NOT NULL,
    type              TEXT          NOT NULL,
    status            worker_status NOT NULL DEFAULT 'online',
    last_seen         TIMESTAMPTZ   DEFAULT NOW(),
    orders_processed  INTEGER       DEFAULT 0
);


-- Create table for storing all order details.
CREATE TABLE orders (
    id                SERIAL        PRIMARY KEY,
    created_at        TIMESTAMPTZ   NOT NULL    DEFAULT NOW(),
    updated_at        TIMESTAMPTZ   NOT NULL    DEFAULT NOW(),
    number            TEXT          UNIQUE NOT NULL,
    customer_name     TEXT          NOT NULL,
    type              order_type    Not NULL,
    table_number      INTEGER,
    delivery_address  TEXT,
    total_amount      NUMERIC(10,2) NOT NULL    CHECK (total_amount >= 0.01),
    priority          INTEGER       NOT NULL    DEFAULT 1   CHECK (priority IN (1,5,10)),
    status            order_status  NOT NULL    DEFAULT 'received',
    processed_by      TEXT,
    completed_at      TIMESTAMPTZ,

    -- encode table invariants
    CONSTRAINT orders_dinein_table_ck
    CHECK (type <> 'dine_in' OR (table_number IS NOT NULL AND table_number BETWEEN 1 AND 100)),

    CONSTRAINT orders_delivery_address_ck
    CHECK (type <> 'delivery' OR (delivery_address IS NOT NULL AND LENGTH(delivery_address) >= 10)),

    CONSTRAINT orders_dinein_no_address_ck
    CHECK (NOT (type = 'dine_in' AND delivery_address IS NOT NULL)),

    CONSTRAINT orders_delivery_no_table_ck
    CHECK (NOT (type = 'delivery' AND table_number IS NOT NULL)),

    CONSTRAINT orders_takeout_neither_table_nor_address_ck
    CHECK (type <> 'takeout' OR (table_number IS NULL AND delivery_address IS NULL)),

    CONSTRAINT orders_number_fmt_ck
    CHECK (number ~ '^ORD_[0-9]{8}_[0-9]{3}$')
);


-- Create a table for a per-day order number counter.
CREATE TABLE order_number_seq (
    day date PRIMARY KEY,
    n   integer NOT NULL
);


-- Create table for storing the individual items associated with each order.
CREATE TABLE order_items (
    id          SERIAL        PRIMARY KEY,
    created_at  TIMESTAMPTZ   NOT NULL    DEFAULT NOW(),
    order_id    INTEGER       NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    name        TEXT          NOT NULL,
    quantity    INTEGER       NOT NULL CHECK (quantity >= 1),
    price       NUMERIC(8,2)  NOT NULL CHECK (price >= 0.01)
);


-- Create table for storing an audit trail of an order's lifecycle, starting with the received status.
CREATE TABLE order_status_log (
    id          SERIAL        PRIMARY KEY,
    created_at  TIMESTAMPTZ   NOT NULL    DEFAULT NOW(),
    order_id    INTEGER       NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    status      order_status  NOT NULL, 
    changed_by  TEXT          NOT NULL,
    changed_at  TIMESTAMPTZ   DEFAULT NOW(),
    notes       TEXT
);


-- =============================
-- Functions and triggers
-- =============================

-- Create function that auto-update updated_at atribute.
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at := NOW();
    RETURN NEW;
END; $$;

-- Create a trigger for any orders table update.
CREATE TRIGGER orders_set_updated_at
BEFORE UPDATE ON orders
FOR EACH ROW EXECUTE FUNCTION set_updated_at();


-- =============================
-- Indexes
-- =============================

-- Speeds: SELECT ... FROM order_items WHERE order_id = $1
CREATE INDEX IF NOT EXISTS order_items_order_id_idx
ON order_items (order_id);

-- Speeds: history retrieval ordered by time
CREATE INDEX IF NOT EXISTS order_status_log_order_id_changed_at_idx
ON order_status_log (order_id, changed_at);

-- Commit the transaction.
COMMIT;