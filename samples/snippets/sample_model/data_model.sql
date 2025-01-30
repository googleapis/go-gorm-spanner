
CREATE SEQUENCE IF NOT EXISTS `singers_seq` OPTIONS (sequence_kind = "bit_reversed_positive");
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

CREATE SEQUENCE IF NOT EXISTS `albums_seq` OPTIONS (sequence_kind = "bit_reversed_positive");

CREATE TABLE IF NOT EXISTS `albums` (
    `album_id` INT64 DEFAULT (GET_NEXT_SEQUENCE_VALUE(Sequence albums_seq)),
    `created_at` TIMESTAMP,
    `updated_at` TIMESTAMP,
    `deleted_at` TIMESTAMP,
    `title` STRING(MAX),
    `marketing_budget` FLOAT64,
    `release_date` date,
    `cover_picture` BYTES(MAX),
    `singer_id` INT64,
    CONSTRAINT `fk_singers_albums` FOREIGN KEY (`singer_id`) REFERENCES `singers`(`id`)
) PRIMARY KEY (`album_id`);

CREATE SEQUENCE IF NOT EXISTS `tracks_seq` OPTIONS (sequence_kind = "bit_reversed_positive");

CREATE TABLE IF NOT EXISTS `tracks` (
    `album_id` INT64,
    `track_number` INT64,
    `created_at` TIMESTAMP,
    `updated_at` TIMESTAMP,
    `deleted_at` TIMESTAMP,
    `title` STRING(MAX),
    `sample_rate` FLOAT64,
) PRIMARY KEY (`album_id`,`track_number`), INTERLEAVE IN PARENT albums ON DELETE CASCADE;

CREATE SEQUENCE IF NOT EXISTS `venues_seq` OPTIONS (sequence_kind = "bit_reversed_positive");

CREATE TABLE IF NOT EXISTS `venues` (
    `id`            INT64 DEFAULT (GET_NEXT_SEQUENCE_VALUE(Sequence venues_seq)),
    `created_at`    TIMESTAMP,
    `updated_at`    TIMESTAMP,
    `deleted_at`    TIMESTAMP,
    `name`          STRING(100),
    `venue_details` JSON,
) PRIMARY KEY (`id`);

CREATE SEQUENCE IF NOT EXISTS `concerts_seq` OPTIONS (sequence_kind = "bit_reversed_positive");

CREATE TABLE IF NOT EXISTS `concerts` (
    `id`            INT64 DEFAULT (GET_NEXT_SEQUENCE_VALUE(Sequence concerts_seq)),
    `created_at`    TIMESTAMP,
    `updated_at`    TIMESTAMP,
    `deleted_at`    TIMESTAMP,
    `venue_id`  INT64 NOT NULL,
    `start_time` TIMESTAMP NOT NULL,
    `end_time` TIMESTAMP NOT NULL,
    `singer_id`  INT64 NOT NULL,
    `name`     STRING(200),
    CONSTRAINT `fk_concerts_venues` FOREIGN KEY (`venue_id`) REFERENCES `venues` (`id`),
    CONSTRAINT `fk_concerts_singers` FOREIGN KEY (`singer_id`) REFERENCES `singers` (`id`),
) PRIMARY KEY (`id`);

CREATE SEQUENCE IF NOT EXISTS `performances_seq` OPTIONS (sequence_kind = "bit_reversed_positive");

CREATE TABLE IF NOT EXISTS `performances` (
    `id` INT64 DEFAULT (GET_NEXT_SEQUENCE_VALUE(Sequence performances_seq)),
    `created_at`    TIMESTAMP,
    `updated_at`    TIMESTAMP,
    `deleted_at`    TIMESTAMP,
    `concert_id` INT64 NOT NULL,
    `album_id`          INT64 NOT NULL,
    `track_number`          INT64 NOT NULL,
    `start_time`        TIMESTAMP,
    `rating`           FLOAT64,
    CONSTRAINT `fk_performances_concerts` FOREIGN KEY (`concert_id`) REFERENCES `concerts` (`id`),
    CONSTRAINT `fk_performances_tracks` FOREIGN KEY (`album_id`, `track_number`) REFERENCES `tracks` (`album_id`, `track_number`),
) PRIMARY KEY (`id`);

CREATE SEQUENCE IF NOT EXISTS `ticket_sales_seq` OPTIONS (
    sequence_kind='bit_reversed_positive',
    start_with_counter=1,
    skip_range_min=1,
    skip_range_max=1000000
);

CREATE PROTO BUNDLE (
  examples.concerts.Address,
  examples.concerts.Item,
  examples.concerts.TicketOrder,
  examples.concerts.TicketOrderHistory
);

CREATE TABLE IF NOT EXISTS `ticket_sales` (
    `id` INT64 DEFAULT (GET_NEXT_SEQUENCE_VALUE(Sequence ticket_sales_seq)),
    `created_at`    TIMESTAMP,
    `updated_at`    TIMESTAMP,
    `deleted_at`    TIMESTAMP,
    `customer_name` STRING(MAX) NOT NULL,
    `seats`         ARRAY<STRING(10)>,
    `concert_id`    INT64 NOT NULL,
    `ticket_order`  examples.concerts.TicketOrder,
    CONSTRAINT `fk_ticket_sales_concerts` FOREIGN KEY (`concert_id`) REFERENCES `concerts` (`id`),
) PRIMARY KEY (`id`);
