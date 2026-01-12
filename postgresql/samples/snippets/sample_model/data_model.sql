
ALTER DATABASE db SET spanner.default_sequence_kind='bit_reversed_positive';

CREATE TABLE IF NOT EXISTS singers (
    id serial primary key,
    created_at timestamptz,
    updated_at timestamptz,
    deleted_at timestamptz,
    first_name varchar,
    last_name varchar,
    full_name varchar generated always as (
       case when first_name is null then last_name
            when last_name  is null then first_name
            else first_name || ' ' || last_name
       end) stored,
    active boolean
);

CREATE INDEX IF NOT EXISTS idx_singers_deleted_at ON singers(deleted_at);

CREATE TABLE IF NOT EXISTS albums (
    album_id serial primary key,
    created_at timestamptz,
    updated_at timestamptz,
    deleted_at timestamptz,
    title varchar,
    marketing_budget float8,
    release_date date,
    cover_picture bytea,
    singer_id bigint,
    CONSTRAINT fk_singers_albums FOREIGN KEY (singer_id) REFERENCES singers(id)
);

CREATE TABLE IF NOT EXISTS tracks (
    album_id bigint,
    track_number bigint,
    created_at timestamptz,
    updated_at timestamptz,
    deleted_at timestamptz,
    title varchar,
    sample_rate float8,
    PRIMARY KEY (album_id,track_number)
) INTERLEAVE IN PARENT albums ON DELETE CASCADE;

CREATE TABLE IF NOT EXISTS venues (
    id            serial primary key,
    created_at    timestamptz,
    updated_at    timestamptz,
    deleted_at    timestamptz,
    name          varchar(100),
    venue_details jsonb
);

CREATE TABLE IF NOT EXISTS concerts (
    id         serial primary key,
    created_at timestamptz,
    updated_at timestamptz,
    deleted_at timestamptz,
    venue_id   bigint NOT NULL,
    start_time timestamptz NOT NULL,
    end_time   timestamptz NOT NULL,
    singer_id  bigint NOT NULL,
    name       varchar(200),
    CONSTRAINT fk_concerts_venues FOREIGN KEY (venue_id) REFERENCES venues (id),
    CONSTRAINT fk_concerts_singers FOREIGN KEY (singer_id) REFERENCES singers (id)
);

CREATE TABLE IF NOT EXISTS performances (
    id           serial primary key,
    created_at   timestamptz,
    updated_at   timestamptz,
    deleted_at   timestamptz,
    concert_id   bigint NOT NULL,
    album_id     bigint NOT NULL,
    track_number bigint NOT NULL,
    start_time   timestamptz,
    rating       float8,
    CONSTRAINT fk_performances_concerts FOREIGN KEY (concert_id) REFERENCES concerts (id),
    CONSTRAINT fk_performances_tracks FOREIGN KEY (album_id, track_number) REFERENCES tracks (album_id, track_number)
);

CREATE TABLE IF NOT EXISTS ticket_sales (
    id            serial primary key,
    created_at    timestamptz,
    updated_at    timestamptz,
    deleted_at    timestamptz,
    customer_name varchar NOT NULL,
    seats         varchar(10)[],
    concert_id    bigint NOT NULL,
    ticket_order  bytea,
    CONSTRAINT fk_ticket_sales_concerts FOREIGN KEY (concert_id) REFERENCES concerts (id)
);
