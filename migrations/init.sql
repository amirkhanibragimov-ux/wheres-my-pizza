-- Wrap everything in a transaction to prevent partial table setup.
BEGIN;


-- Clean reset to ensure the clean setup even if prior runs failed (development only!).
DROP TABLE IF EXISTS order_status_log CASCADE;
DROP TABLE IF EXISTS order_items CASCADE;
DROP TABLE IF EXISTS orders CASCADE;
DROP TABLE IF EXISTS workers CASCADE;
DROP TYPE  IF EXISTS order_status;


-- Enumerations.
CREATE TYPE order_status AS ENUM ('received', 'cooking', 'ready', 'completed', 'cancelled');


-- Create function that auto-update updated_at atribute.
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at := NOW();
    RETURN NEW;
END; $$;


-- 1. Table for storing registry for all kitchen workers, their specializations, and their current status.
CREATE TABLE workers (
    id                SERIAL      PRIMARY KEY,
    created_at        TIMESTAMPTZ NOT NULL    DEFAULT NOW(),
    name              TEXT        UNIQUE NOT NULL,
    type              TEXT        NOT NULL,
    status            TEXT        NOT NULL DEFAULT 'online' CHECK (status IN ('online','offline')),
    last_seen         TIMESTAMPTZ DEFAULT NOW(),
    orders_processed  INTEGER     DEFAULT 0
);


-- 2. Table for storing all order details.
CREATE TABLE orders (
    id                SERIAL        PRIMARY KEY,
    created_at        TIMESTAMPTZ   NOT NULL    DEFAULT NOW(),
    updated_at        TIMESTAMPTZ   NOT NULL    DEFAULT NOW(),
    number            TEXT          UNIQUE NOT NULL,
    customer_name     TEXT          NOT NULL,
    type              TEXT          NOT NULL CHECK (type IN ('dine_in', 'takeout', 'delivery')),
    table_number      INTEGER,
    delivery_address  TEXT,
    total_amount      NUMERIC(10,2) NOT NULL CHECK (total_amount >= 0.01),
    priority          INTEGER       DEFAULT 1 CHECK (priority IN (1,5,10)),
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


-- Create a table for a per-day order number counter (UTC day)
CREATE TABLE order_number_seq (
  day date PRIMARY KEY,
  n   integer NOT NULL
);


-- Create a trigger for any orders table update.
CREATE TRIGGER orders_set_updated_at
BEFORE UPDATE ON orders
FOR EACH ROW EXECUTE FUNCTION set_updated_at();


-- 3. Table for storing the individual items associated with each order.
CREATE TABLE order_items (
    id          SERIAL        PRIMARY KEY,
    created_at  TIMESTAMPTZ   NOT NULL    DEFAULT NOW(),
    order_id    INTEGER       REFERENCES orders(id) ON DELETE CASCADE,
    name        TEXT          NOT NULL,
    quantity    INTEGER       NOT NULL CHECK (quantity >= 1),
    price       NUMERIC(8,2)  NOT NULL CHECK (price >= 0.01)
);


-- 4. Table for storing an audit trail of an order's lifecycle, starting with the received status.
CREATE TABLE order_status_log (
    id          SERIAL        PRIMARY KEY,
    created_at  TIMESTAMPTZ   NOT NULL    DEFAULT NOW(),
    order_id    INTEGER       NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    status      order_status  NOT NULL, 
    changed_by  TEXT,
    changed_at  TIMESTAMPTZ   DEFAULT NOW(),
    notes       TEXT
);


-- Commit the transaction.
COMMIT;