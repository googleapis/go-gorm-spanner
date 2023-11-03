-- Copyright 2023 Google LLC.
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

CREATE SEQUENCE IF NOT EXISTS singers_seq OPTIONS (sequence_kind = "bit_reversed_positive");
CREATE TABLE IF NOT EXISTS `singers` (
    `id` INT64 DEFAULT (GET_NEXT_SEQUENCE_VALUE(Sequence singers_seq)),
    `created_at` TIMESTAMP,
    `updated_at` TIMESTAMP,
    `deleted_at` TIMESTAMP,
    `first_name` STRING(MAX),
    `last_name` STRING(MAX),
    `full_name` STRING(MAX) AS (ARRAY_TO_STRING([first_name, last_name], " ")) STORED,
    `active` BOOL
    ) PRIMARY KEY (`id`);

CREATE INDEX IF NOT EXISTS `idx_singers_deleted_at` ON `singers`(`deleted_at`);

CREATE SEQUENCE IF NOT EXISTS albums_seq OPTIONS (sequence_kind = "bit_reversed_positive");

CREATE TABLE IF NOT EXISTS `albums` (
    `id` INT64 DEFAULT (GET_NEXT_SEQUENCE_VALUE(Sequence albums_seq)),
    `created_at` TIMESTAMP,
    `updated_at` TIMESTAMP,
    `deleted_at` TIMESTAMP,
    `title` STRING(MAX),
    `marketing_budget` FLOAT64,
    `release_date` date,
    `cover_picture` BYTES(MAX),
    `singer_id` INT64,
    CONSTRAINT `fk_singers_albums` FOREIGN KEY (`singer_id`) REFERENCES `singers`(`id`)
) PRIMARY KEY (`id`);

CREATE SEQUENCE IF NOT EXISTS tracks_seq OPTIONS (sequence_kind = "bit_reversed_positive");

CREATE TABLE IF NOT EXISTS `tracks` (
    `id` INT64 DEFAULT (GET_NEXT_SEQUENCE_VALUE(Sequence tracks_seq)),
    `created_at` TIMESTAMP,
    `updated_at` TIMESTAMP,
    `deleted_at` TIMESTAMP,
    `track_number` INT64,
    `title` STRING(MAX),
    `sample_rate` FLOAT64,
    ) PRIMARY KEY (`id`,`track_number`), INTERLEAVE IN PARENT albums ON DELETE CASCADE;
