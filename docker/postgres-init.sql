-- Initialize sMAP Archiver Database
CREATE EXTENSION IF NOT EXISTS hstore;

-- From python/smap/archiver/sql/tables.psql

-- table for list of sMAP sources we should be subscribed to
CREATE TABLE IF NOT EXISTS subscription (
       id INT PRIMARY KEY,
       uuid VARCHAR(36),
       url VARCHAR(512) NOT NULL,
       resource VARCHAR(512) NOT NULL DEFAULT '/+',
       key VARCHAR(36),
       public BOOLEAN DEFAULT true,
       description VARCHAR(256),
       owner_id integer DEFAULT 1
);
CREATE UNIQUE INDEX IF NOT EXISTS subscription_key_ind ON subscription(key);
CREATE SEQUENCE IF NOT EXISTS subscription_id_seq;
ALTER TABLE subscription ALTER COLUMN id SET DEFAULT NEXTVAL('subscription_id_seq');

-- list of streams associated with a sMAP source
CREATE TABLE IF NOT EXISTS stream (
       id INT PRIMARY KEY,
       subscription_id INT NOT NULL,
       uuid VARCHAR(36) UNIQUE,
       metadata HSTORE DEFAULT hstore(array[]::varchar[]),

       FOREIGN KEY (subscription_id) REFERENCES subscription(id)
         ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS uuid_ind ON stream(uuid);
CREATE INDEX IF NOT EXISTS subscription_int ON stream(subscription_id);
CREATE INDEX IF NOT EXISTS metadata_index ON stream USING GIST(metadata);
CREATE SEQUENCE IF NOT EXISTS stream_id_seq;
ALTER TABLE stream ALTER COLUMN id SET DEFAULT NEXTVAL('stream_id_seq');

-- create a set of API keys and permissions
CREATE TABLE IF NOT EXISTS permission (
       id INT PRIMARY KEY,
       key VARCHAR(36),
       description VARCHAR(256),
        
       valid_after TIMESTAMP DEFAULT NULL,
       valid_until TIMESTAMP DEFAULT NULL,
       can_select BOOLEAN NOT NULL DEFAULT true,
       can_delete BOOLEAN NOT NULL DEFAULT false,
       can_set BOOLEAN NOT NULL DEFAULT false
);
CREATE SEQUENCE IF NOT EXISTS permission_id_seq;
ALTER TABLE permission ALTER COLUMN id SET DEFAULT NEXTVAL('permission_id_seq');

-- contains permission -> set(subscription) that contains the
-- subscriptions that the permission applies to
CREATE TABLE IF NOT EXISTS permission_subscriptions (
       id INT PRIMARY KEY,
       subscription_id INT,
       permission_id INT,

       UNIQUE (permission_id, subscription_id),
       FOREIGN KEY (subscription_id) REFERENCES subscription(id),
       FOREIGN KEY (permission_id) REFERENCES permission(id)
);
CREATE SEQUENCE IF NOT EXISTS permission_subscriptions_id_seq;
ALTER TABLE permission_subscriptions ALTER COLUMN id SET DEFAULT NEXTVAL('permission_subscriptions_id_seq');

-- From python/smap/archiver/sql/insert-stream.psql

CREATE OR REPLACE FUNCTION add_stream(subscription INT, uid VARCHAR(64)) RETURNS INT AS
$$
DECLARE
    existing_id INT;
    existing_sub INT;
BEGIN
    -- if the stream is already in there, avoid burning a sequence number.
    SELECT id, subscription_id INTO existing_id, existing_sub
    FROM stream 
    WHERE uuid = uid;
    IF existing_id IS NOT NULL THEN
       -- don't allow duplicate uuids with different keys.
       IF existing_sub = subscription THEN
          RETURN existing_id;
       ELSE
          RAISE EXCEPTION 'UUID already claimed by different API key: %', uid;
       END IF;
    ELSE
        INSERT INTO stream(subscription_id, uuid) VALUES (subscription, uid);
        RETURN CURRVAL('stream_id_seq');
    END IF;
END;
$$
LANGUAGE plpgsql;

-- From python/smap/archiver/sql/republish.psql
CREATE TABLE IF NOT EXISTS republish (
       inserted_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
       key VARCHAR(36),
       obj TEXT,
       sent BOOLEAN DEFAULT FALSE
);

-- Seed a default API key for easier out-of-the-box experience
INSERT INTO subscription (id, uuid, url, resource, key) 
VALUES (1, '00000000-0000-0000-0000-000000000000', 'http://localhost', '/+', 'mykey')
ON CONFLICT DO NOTHING;

-- Adjust sequence if we manually inserted id=1
SELECT setval('subscription_id_seq', COALESCE((SELECT MAX(id) FROM subscription), 1));

-- Create 'data' table for time-series readings (replacing readingdb)
CREATE TABLE IF NOT EXISTS data (
    sid INT NOT NULL REFERENCES stream(id) ON DELETE CASCADE,
    time BIGINT NOT NULL,
    value DOUBLE PRECISION NOT NULL
);

-- Optimize with an index
CREATE INDEX IF NOT EXISTS data_sid_time_idx ON data (sid, time);

-- If TimescaleDB extension is available, convert to hypertable
-- (Safe to run multiple times, will error gracefully if already a hypertable or if timescale is missing)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
        PERFORM create_hypertable('data', 'time', chunk_time_interval => 86400000, if_not_exists => TRUE);
    END IF;
EXCEPTION
    WHEN OTHERS THEN
        RAISE NOTICE 'TimescaleDB hypertable creation skipped: %', SQLERRM;
END $$;
