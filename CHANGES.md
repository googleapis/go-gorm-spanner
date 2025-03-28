# Changelog

## [1.8.0](https://github.com/googleapis/go-gorm-spanner/compare/v1.7.0...v1.8.0) (2025-03-28)


### Features

* Isolation level repeatable read ([#159](https://github.com/googleapis/go-gorm-spanner/issues/159)) ([44fee75](https://github.com/googleapis/go-gorm-spanner/commit/44fee75b8888b137ed8b64c01c8f1300c6450501))

## [1.7.0](https://github.com/googleapis/go-gorm-spanner/compare/v1.6.0...v1.7.0) (2025-03-10)


### Features

* Use IDENTITY for generated primary keys ([#150](https://github.com/googleapis/go-gorm-spanner/issues/150)) ([7e47047](https://github.com/googleapis/go-gorm-spanner/commit/7e47047f2a4ca7a9e899002dc734be70d853e7e5))


### Performance Improvements

* Use last_statement for auto-commit ([#154](https://github.com/googleapis/go-gorm-spanner/issues/154)) ([bd5a253](https://github.com/googleapis/go-gorm-spanner/commit/bd5a253d85e233d91b6cea98abdcc7742f2706c0))

## [1.6.0](https://github.com/googleapis/go-gorm-spanner/compare/v1.5.0...v1.6.0) (2025-02-17)


### Features

* Add native array data types ([#144](https://github.com/googleapis/go-gorm-spanner/issues/144)) ([6ecbebd](https://github.com/googleapis/go-gorm-spanner/commit/6ecbebd2973531414b78d1c43771ce452052a06a))
* Support custom Spanner configurations ([#136](https://github.com/googleapis/go-gorm-spanner/issues/136)) ([abc6937](https://github.com/googleapis/go-gorm-spanner/commit/abc69376fbda7def0862ce7e278312cefec3b9ae))


### Bug Fixes

* Return clear error message for unique constraints ([#138](https://github.com/googleapis/go-gorm-spanner/issues/138)) ([e97f49b](https://github.com/googleapis/go-gorm-spanner/commit/e97f49b1b4a625adf33964fa78114bc6d06b7cda))


### Documentation

* Add sample for using protobuf columns ([#140](https://github.com/googleapis/go-gorm-spanner/issues/140)) ([ea1290a](https://github.com/googleapis/go-gorm-spanner/commit/ea1290ac8d06fa149f783b81dee7103f65083ef7))
* Add samples and documentation for all data types ([#146](https://github.com/googleapis/go-gorm-spanner/issues/146)) ([29004ad](https://github.com/googleapis/go-gorm-spanner/commit/29004ad6dda7d92762a3684408cbf49d8df06f6c)), refs [#131](https://github.com/googleapis/go-gorm-spanner/issues/131)
* Add test to verify that FOR UPDATE clauses can be used ([#147](https://github.com/googleapis/go-gorm-spanner/issues/147)) ([9487235](https://github.com/googleapis/go-gorm-spanner/commit/9487235ea77b4eabf1180e958774e8d268fa7280))
* Update OnConflict limitations ([#148](https://github.com/googleapis/go-gorm-spanner/issues/148)) ([b635a27](https://github.com/googleapis/go-gorm-spanner/commit/b635a270c4e9bff3f581eab9b5ffe85e1acb145d))

## [1.5.0](https://github.com/googleapis/go-gorm-spanner/compare/v1.4.0...v1.5.0) (2025-01-20)


### Features

* Add sample code snippets for common Spanner and gorm features ([#123](https://github.com/googleapis/go-gorm-spanner/issues/123)) ([8ff4040](https://github.com/googleapis/go-gorm-spanner/commit/8ff4040de016d9e03bef66b439371ee2f45ee986))
* Add transaction runner ([#127](https://github.com/googleapis/go-gorm-spanner/issues/127)) ([b446423](https://github.com/googleapis/go-gorm-spanner/commit/b446423dfb01e1c4bc76a59a3bf98a445b899465))
* Support dry-run mode for auto-migrate ([#124](https://github.com/googleapis/go-gorm-spanner/issues/124)) ([abc7c9b](https://github.com/googleapis/go-gorm-spanner/commit/abc7c9b340a2b3127ee8e8849bb4366bd1254f8d))


### Bug Fixes

* Avoid getting column details from other tables ([#137](https://github.com/googleapis/go-gorm-spanner/issues/137)) ([0aac7e5](https://github.com/googleapis/go-gorm-spanner/commit/0aac7e5daf311d9783b536dbffde93de6a47f67a))

## [1.4.0](https://github.com/googleapis/go-gorm-spanner/compare/v1.3.0...v1.4.0) (2024-10-21)


### Features

* INSERT OR UPDATE ([#28](https://github.com/googleapis/go-gorm-spanner/issues/28)) ([df84b30](https://github.com/googleapis/go-gorm-spanner/commit/df84b306affc215a268dbddbc1f3af4ce0fb7e55))

## [1.3.0](https://github.com/googleapis/go-gorm-spanner/compare/v1.2.2...v1.3.0) (2024-08-23)


### Features

* Support migrator.GetIndexes ([#112](https://github.com/googleapis/go-gorm-spanner/issues/112)) ([08fa9fa](https://github.com/googleapis/go-gorm-spanner/commit/08fa9fa01f3f8db2737d38a1deba395d48e61524)), refs [#69](https://github.com/googleapis/go-gorm-spanner/issues/69)

## [1.2.2](https://github.com/googleapis/go-gorm-spanner/compare/v1.2.1...v1.2.2) (2024-06-03)


### Bug Fixes

* **deps:** Update google.golang.org/genproto digest to 5315273 ([#91](https://github.com/googleapis/go-gorm-spanner/issues/91)) ([81fb3d8](https://github.com/googleapis/go-gorm-spanner/commit/81fb3d86ed35e9c434f1bc6a0bb627dd3c6723d4))
* **deps:** Update module cloud.google.com/go to v0.114.0 ([#87](https://github.com/googleapis/go-gorm-spanner/issues/87)) ([5ab230d](https://github.com/googleapis/go-gorm-spanner/commit/5ab230d32f45d1bdb6038ebac26583d16d20f19f))
* **deps:** Update module cloud.google.com/go/longrunning to v0.5.7 ([#83](https://github.com/googleapis/go-gorm-spanner/issues/83)) ([7379858](https://github.com/googleapis/go-gorm-spanner/commit/7379858d7c968be44edfde6a7fd410f0983f9ad8))
* **deps:** Update module cloud.google.com/go/spanner to v1.63.0 ([#85](https://github.com/googleapis/go-gorm-spanner/issues/85)) ([55051a4](https://github.com/googleapis/go-gorm-spanner/commit/55051a4f967b3f227d4338e8a6537bfa4a7e9042))
* **deps:** Update module github.com/docker/docker to v26.1.3+incompatible ([#80](https://github.com/googleapis/go-gorm-spanner/issues/80)) ([a1dc3a7](https://github.com/googleapis/go-gorm-spanner/commit/a1dc3a79647002137f3acb77661a041d81d34f75))
* **deps:** Update module github.com/googleapis/go-gorm-spanner to v1.2.1 ([#76](https://github.com/googleapis/go-gorm-spanner/issues/76)) ([9ef0ef0](https://github.com/googleapis/go-gorm-spanner/commit/9ef0ef0b54f51ba3e42a2c898c7ed90cd875b3c0))
* **deps:** Update module github.com/googleapis/go-sql-spanner to v1.3.1 ([#77](https://github.com/googleapis/go-gorm-spanner/issues/77)) ([720aaf7](https://github.com/googleapis/go-gorm-spanner/commit/720aaf7db6e823a24b58eacefe8e499f22835a20))
* **deps:** Update module github.com/googleapis/go-sql-spanner to v1.4.0 ([#98](https://github.com/googleapis/go-gorm-spanner/issues/98)) ([735d275](https://github.com/googleapis/go-gorm-spanner/commit/735d275add5f6cfba76a84523f2d2d87e21ce31f))
* **deps:** Update module github.com/shopspring/decimal to v1.4.0 ([#86](https://github.com/googleapis/go-gorm-spanner/issues/86)) ([8585951](https://github.com/googleapis/go-gorm-spanner/commit/8585951eb744b6761b6d5568ace429a85e391275))
* **deps:** Update module google.golang.org/api to v0.182.0 ([#88](https://github.com/googleapis/go-gorm-spanner/issues/88)) ([e4cb401](https://github.com/googleapis/go-gorm-spanner/commit/e4cb401573af42cefa7bdc2ac759cf19a0a5a2e8))
* **deps:** Update module google.golang.org/grpc to v1.64.0 ([#89](https://github.com/googleapis/go-gorm-spanner/issues/89)) ([fa899c8](https://github.com/googleapis/go-gorm-spanner/commit/fa899c841b4979db98273dbe62794ca727d07e53))
* **deps:** Update module google.golang.org/protobuf to v1.34.1 ([#90](https://github.com/googleapis/go-gorm-spanner/issues/90)) ([1395b27](https://github.com/googleapis/go-gorm-spanner/commit/1395b2778a46e5f1c4616b3d8a09eab155722b6f))
* **deps:** Update module gorm.io/gorm to v1.25.10 ([#79](https://github.com/googleapis/go-gorm-spanner/issues/79)) ([3bc56b8](https://github.com/googleapis/go-gorm-spanner/commit/3bc56b84f54dc1aaff68f8941e4c5323bf544e4c))

## [1.2.1](https://github.com/googleapis/go-gorm-spanner/compare/v1.2.0...v1.2.1) (2024-04-20)


### Bug Fixes

* **deps:** Update google.golang.org/genproto digest to 8c6c420 ([#67](https://github.com/googleapis/go-gorm-spanner/issues/67)) ([367ab93](https://github.com/googleapis/go-gorm-spanner/commit/367ab9390310a662ed975c73fcbc1b73a5ccdb80))
* **deps:** Update google.golang.org/genproto digest to c3f9821 ([#59](https://github.com/googleapis/go-gorm-spanner/issues/59)) ([682f128](https://github.com/googleapis/go-gorm-spanner/commit/682f128db2063ff0176b5e9620c2e2cd3c43dca6))
* **deps:** Update module cloud.google.com/go to v0.112.2 ([#60](https://github.com/googleapis/go-gorm-spanner/issues/60)) ([04944ab](https://github.com/googleapis/go-gorm-spanner/commit/04944ab35bc860ead24d697551dac2f143e62917))
* **deps:** Update module github.com/docker/docker to v26 ([#64](https://github.com/googleapis/go-gorm-spanner/issues/64)) ([7edc78a](https://github.com/googleapis/go-gorm-spanner/commit/7edc78a3b3de8698fa871b625a400ad3f36ccff3))
* **deps:** Update module github.com/googleapis/go-gorm-spanner to v1 ([#65](https://github.com/googleapis/go-gorm-spanner/issues/65)) ([e024a54](https://github.com/googleapis/go-gorm-spanner/commit/e024a54e2439b764a4ccd927aa4e8b4b6b2ba132))
* **deps:** Update module google.golang.org/api to v0.172.0 ([#62](https://github.com/googleapis/go-gorm-spanner/issues/62)) ([e005a71](https://github.com/googleapis/go-gorm-spanner/commit/e005a71ee7db99d146ccee31ef960fd7512abf19))
* **deps:** Update module google.golang.org/grpc to v1.63.0 ([#63](https://github.com/googleapis/go-gorm-spanner/issues/63)) ([79fec61](https://github.com/googleapis/go-gorm-spanner/commit/79fec61955d18226a9387e22f72a83670c2f9534))
* **deps:** Update module gorm.io/gorm to v1.25.9 ([#61](https://github.com/googleapis/go-gorm-spanner/issues/61)) ([4606bf1](https://github.com/googleapis/go-gorm-spanner/commit/4606bf180729fb88b388f9ce0e08f2c3943b9ee7))

## [1.2.0](https://github.com/googleapis/go-gorm-spanner/compare/v1.1.0...v1.2.0) (2024-03-25)


### Features

* Add support for float32 ([#54](https://github.com/googleapis/go-gorm-spanner/issues/54)) ([7f30933](https://github.com/googleapis/go-gorm-spanner/commit/7f3093362687658ba50ad8fd19d9c363d932f4cf))


### Bug Fixes

* **deps:** Update google.golang.org/genproto digest to 6e1732d ([#51](https://github.com/googleapis/go-gorm-spanner/issues/51)) ([ce2a60e](https://github.com/googleapis/go-gorm-spanner/commit/ce2a60eedd77a7145be3d10fa665795d87a3a483))
* **deps:** Update google.golang.org/genproto digest to 94a12d6 ([#55](https://github.com/googleapis/go-gorm-spanner/issues/55)) ([82faa60](https://github.com/googleapis/go-gorm-spanner/commit/82faa60205d7428c7007c82d940753d87d8b2a0a))
* **deps:** Update google.golang.org/genproto digest to b0ce06b ([#33](https://github.com/googleapis/go-gorm-spanner/issues/33)) ([fcecf30](https://github.com/googleapis/go-gorm-spanner/commit/fcecf306c5754d6dafcbcda12027dc73f817fcb3))
* **deps:** Update google.golang.org/genproto digest to c811ad7 ([#40](https://github.com/googleapis/go-gorm-spanner/issues/40)) ([550e590](https://github.com/googleapis/go-gorm-spanner/commit/550e5905a6c0ba78253c0be39528385ef5596f08))
* **deps:** Update module cloud.google.com/go to v0.112.1 ([#41](https://github.com/googleapis/go-gorm-spanner/issues/41)) ([abc779e](https://github.com/googleapis/go-gorm-spanner/commit/abc779eaf6dd51539a929f302a4f105717d950c2))
* **deps:** Update module cloud.google.com/go/longrunning to v0.5.6 ([#52](https://github.com/googleapis/go-gorm-spanner/issues/52)) ([4230aac](https://github.com/googleapis/go-gorm-spanner/commit/4230aac0ce07a16e3b23ea443d3f2e44d1b5f55d))
* **deps:** Update module cloud.google.com/go/spanner to v1.57.0 ([#38](https://github.com/googleapis/go-gorm-spanner/issues/38)) ([dc07c18](https://github.com/googleapis/go-gorm-spanner/commit/dc07c18a672cdae74e7dbc36725023bd3a9a134e))
* **deps:** Update module cloud.google.com/go/spanner to v1.59.0 ([#45](https://github.com/googleapis/go-gorm-spanner/issues/45)) ([beadaeb](https://github.com/googleapis/go-gorm-spanner/commit/beadaeb25e1cd65b2185900f6b05fe7a7586e9a2))
* **deps:** Update module cloud.google.com/go/spanner to v1.60.0 ([#57](https://github.com/googleapis/go-gorm-spanner/issues/57)) ([cc431b8](https://github.com/googleapis/go-gorm-spanner/commit/cc431b80b998e05a233a435c666e0d6e11e5d765))
* **deps:** Update module github.com/docker/docker to v24.0.9+incompatible ([#34](https://github.com/googleapis/go-gorm-spanner/issues/34)) ([14a5b21](https://github.com/googleapis/go-gorm-spanner/commit/14a5b214676c7b0584462e48689e160dfaecf7f3))
* **deps:** Update module github.com/docker/go-connections to v0.5.0 ([#36](https://github.com/googleapis/go-gorm-spanner/issues/36)) ([0b2f6ff](https://github.com/googleapis/go-gorm-spanner/commit/0b2f6ff10ff03ed2b003e8fcea4ef83edd1eba77))
* **deps:** Update module github.com/googleapis/go-sql-spanner to v1.3.0 ([#58](https://github.com/googleapis/go-gorm-spanner/issues/58)) ([e36d37a](https://github.com/googleapis/go-gorm-spanner/commit/e36d37abd151346e80f51858021896c953b96ea9))
* **deps:** Update module github.com/stretchr/testify to v1.9.0 ([#42](https://github.com/googleapis/go-gorm-spanner/issues/42)) ([e9062a3](https://github.com/googleapis/go-gorm-spanner/commit/e9062a36f9f476299e271f13578356829474df46))
* **deps:** Update module google.golang.org/api to v0.169.0 ([#43](https://github.com/googleapis/go-gorm-spanner/issues/43)) ([8870a7f](https://github.com/googleapis/go-gorm-spanner/commit/8870a7f122fbc8335a993e4292eb6ee7dc2636e3))
* **deps:** Update module google.golang.org/grpc to v1.62.0 ([#37](https://github.com/googleapis/go-gorm-spanner/issues/37)) ([6ef1a41](https://github.com/googleapis/go-gorm-spanner/commit/6ef1a41423438562026e5e819606225c225f1016))
* **deps:** Update module google.golang.org/grpc to v1.62.1 ([#44](https://github.com/googleapis/go-gorm-spanner/issues/44)) ([4ba1696](https://github.com/googleapis/go-gorm-spanner/commit/4ba1696045fb88bd400655b672e607e93a4389d3))
* **deps:** Update module gorm.io/gorm to v1.25.7 ([#35](https://github.com/googleapis/go-gorm-spanner/issues/35)) ([cd7de7b](https://github.com/googleapis/go-gorm-spanner/commit/cd7de7bcb09bf54c38d76bd17471f65fede6ab0f))
* **deps:** Update module gorm.io/gorm to v1.25.8 ([#56](https://github.com/googleapis/go-gorm-spanner/issues/56)) ([65a6533](https://github.com/googleapis/go-gorm-spanner/commit/65a6533e554536e4d011295c2205ff9ac69862a3))

## [1.1.0](https://github.com/googleapis/go-gorm-spanner/compare/v1.0.0...v1.1.0) (2024-02-06)


### Features

* Commit timestamp fields ([#32](https://github.com/googleapis/go-gorm-spanner/issues/32)) ([aef6dfa](https://github.com/googleapis/go-gorm-spanner/commit/aef6dfaacc938287b7b2b2eb9c1ef36565967e2a))
* Support AutoMigrate for modifying existing tables ([#27](https://github.com/googleapis/go-gorm-spanner/issues/27)) ([cb7163b](https://github.com/googleapis/go-gorm-spanner/commit/cb7163b05a6bbfee022de63d5b09f98ea0f75830))


### Bug Fixes

* **deps:** Update github.com/googleapis/go-gorm-spanner digest to cb7163b ([#29](https://github.com/googleapis/go-gorm-spanner/issues/29)) ([1e345fe](https://github.com/googleapis/go-gorm-spanner/commit/1e345fed705fca89a55c07acc9d06c39fac55962))
* **deps:** Update github.com/googleapis/go-gorm-spanner digest to e5c0b83 ([#11](https://github.com/googleapis/go-gorm-spanner/issues/11)) ([dde97ac](https://github.com/googleapis/go-gorm-spanner/commit/dde97ac4a11b3ca1ca1ad7307a658952314cde70))
* **deps:** Update github.com/googleapis/go-sql-spanner digest to 4aa18f6 ([#12](https://github.com/googleapis/go-gorm-spanner/issues/12)) ([22da32d](https://github.com/googleapis/go-gorm-spanner/commit/22da32dc2fe901066b4b767dc11d34dd64339249))
* **deps:** Update google.golang.org/genproto digest to 1f4bbc5 ([#14](https://github.com/googleapis/go-gorm-spanner/issues/14)) ([0b9d283](https://github.com/googleapis/go-gorm-spanner/commit/0b9d283967c4d76298a3f588d5526f751ec62cb1))
* **deps:** Update module cloud.google.com/go to v0.112.0 ([#15](https://github.com/googleapis/go-gorm-spanner/issues/15)) ([3d24efc](https://github.com/googleapis/go-gorm-spanner/commit/3d24efce75fe9cfbe22375680673c0f2d0c0a957))
* **deps:** Update module cloud.google.com/go/longrunning to v0.5.5 ([#16](https://github.com/googleapis/go-gorm-spanner/issues/16)) ([4b1c0a7](https://github.com/googleapis/go-gorm-spanner/commit/4b1c0a7ca33ca0bed04379991073975b29fe22e3))
* **deps:** Update module google.golang.org/api to v0.161.0 ([#17](https://github.com/googleapis/go-gorm-spanner/issues/17)) ([6469d03](https://github.com/googleapis/go-gorm-spanner/commit/6469d03a2180920eed5e49c011b2d3b8488caeaa))
