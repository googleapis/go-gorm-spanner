-- Copyright 2026 Google LLC.
--
-- Licensed under the Apache License, Version 2.0 (the "License");
-- you may not use this file except in compliance with the License.
-- You may obtain a copy of the License at
--
--      http://www.apache.org/licenses/LICENSE-2.0
--
-- Unless required by applicable law or agreed to in writing, software
-- distributed under the License is distributed on an "AS IS" BASIS,
-- WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
-- See the License for the specific language governing permissions and
-- limitations under the License.

alter database db set spanner.default_sequence_kind = 'bit_reversed_positive';

CREATE TABLE IF NOT EXISTS singers (
    id serial primary key,
    created_at timestamptz,
    updated_at timestamptz,
    deleted_at timestamptz,
    first_name varchar,
    last_name  varchar,
    full_name  varchar generated always as (CASE WHEN first_name IS NULL THEN last_name WHEN last_name  IS NULL THEN first_name ELSE first_name || ' ' || last_name END) stored,
    active     boolean
);

CREATE INDEX IF NOT EXISTS idx_singers_deleted_at ON singers(deleted_at);

CREATE TABLE IF NOT EXISTS albums (
    id serial primary key,
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
    id serial,
    created_at timestamptz,
    updated_at timestamptz,
    deleted_at timestamptz,
    track_number bigint,
    title varchar,
    sample_rate float8,
    primary key (id, track_number)
) INTERLEAVE IN PARENT albums ON DELETE CASCADE;
