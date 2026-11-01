# Changelog

## [0.4.0](https://github.com/getsphericon/sphericon/compare/v0.3.5...v0.4.0) (2026-11-01)


### ⚠ BREAKING CHANGES

* **tracking:** the Sending domain resource on /site and /api no longer carries `trackingRecord`; the Tracking domain has its own operations (`/tracking-domains`), the Permission `tracking_domains:manage` and the scopes `tracking_domains:read` and `tracking_domains:write`. Upgrade: run `sphericon upgrade`; hosts verified as track.<sending domain> are logged as removed and not moved, so set the Tracking domain again on the Tracking settings page.
* **integrations:** the table send_limiters is renamed to provider_limiters. Upgrade: nothing to do; the migration renames the table in place and keeps its rows.
* **condition:** the performed and notPerformed operators and the "event:<action>" field of the rule leaf are gone from the /site and /api contracts, in favour of the "event" leaf. Upgrade: rewrite each stored or sent event condition as an "event" leaf (performed is atLeast 1, notPerformed is atMost 0, the day window moves to withinDays); a dev database reseeds with mise run db:seed.
* the `activate` action of Automations and Rules is replaced by `publish` on `/site`, `/api` and MCP, the scope `automations:activate` is renamed `automations:publish`, an Automation and a Rule carry their definition in `live` and `draft` versions instead of top-level fields, and the status `draft` of Automations and Rules is replaced by `inactive`. Upgrade: run `sphericon upgrade` from the previous release without skipping it; the same run moves stored tokens to the new scope and the definitions into versions before it drops the old fields, so swap the binary right after the migration. API clients call `publish` instead of `activate` and read the definition from `live` and `draft`.
* **config:** SPHERICON_SIGNUP_SURVEY, SPHERICON_REQUIRE_EMAIL_VERIFICATION, SPHERICON_TELEMETRY and SPHERICON_RELEASE_CHECK are gone; SPHERICON_MODE decides what they did. Upgrade: delete those variables from the environment. An Instance with no SPHERICON_MODE is selfhosted: it no longer requires email verification, asks no Sign-up survey and checks for releases. To stop the Telemetry report set DO_NOT_TRACK=1 or switch it off as the Owner in the app.

### Features

* a live Automation or Rule changes only by Publish, a scheduled Broadcast keeps its audience ([#775](https://github.com/getsphericon/sphericon/issues/775)) ([e0445f7](https://github.com/getsphericon/sphericon/commit/e0445f73970af12a08b3683926a6d735e670df42))
* **account:** a User time zone for the app's times and the User's mail (ADR 0068) ([#812](https://github.com/getsphericon/sphericon/issues/812)) ([3c73ac4](https://github.com/getsphericon/sphericon/commit/3c73ac427fd2ec7c82c01749e28227dd4fc2ba55))
* **account:** change the email and the password in modals ([136ccd8](https://github.com/getsphericon/sphericon/commit/136ccd85455a19624788ca24701a0da35e498b7f))
* **account:** delete the user from a modal with its inputs ([dded6b7](https://github.com/getsphericon/sphericon/commit/dded6b700f0335264a5c997e36e33ada014cbc00))
* **account:** manage the second factor and passkeys in modals ([44c2e80](https://github.com/getsphericon/sphericon/commit/44c2e804f5c77dd6e2e05aeabade2c544ccbd655))
* **account:** one Connect or Disconnect button per sign-in provider ([5a70e4e](https://github.com/getsphericon/sphericon/commit/5a70e4e0844011e9d44a9b4a426c9f4156300b06))
* **accounts:** sign-up follows the Instance Mode, closed after the first User on selfhosted ([#690](https://github.com/getsphericon/sphericon/issues/690)) ([b694182](https://github.com/getsphericon/sphericon/commit/b69418288492e4f49752edeff143ab594de8eca1))
* **analytics:** the reports open on the last 7 days ([#707](https://github.com/getsphericon/sphericon/issues/707)) ([9e0964b](https://github.com/getsphericon/sphericon/commit/9e0964bebb109c9458fa22994e339f4c5df7b0d8))
* **api:** contacts:export and suppressions:write scopes mirror their Permissions ([#778](https://github.com/getsphericon/sphericon/issues/778)) ([07f70b3](https://github.com/getsphericon/sphericon/commit/07f70b306c3cf883796afdd336da2b2b5bc6fd98))
* **api:** personal tokens, a member's own credential for the /api surface (ADR 0061) ([#797](https://github.com/getsphericon/sphericon/issues/797)) ([47e27f4](https://github.com/getsphericon/sphericon/commit/47e27f48137cbd0f1e244479babf27d92edee4a8))
* **audit:** filter the audit log by the values it holds; retention opens in a modal ([de9ca99](https://github.com/getsphericon/sphericon/commit/de9ca990192ba26555b7c71b881ca0cfc4e4012d))
* **audit:** the export sits beside the audit log page title ([d72bbaa](https://github.com/getsphericon/sphericon/commit/d72bbaa58d12b9377edaf675636152ef3051e598))
* **auth:** enter the authenticator code in one box per digit ([6d35cb1](https://github.com/getsphericon/sphericon/commit/6d35cb12e9c61861e40adff565d294730872f403))
* **automations:** edit http step headers as rows and dry-run its body ([#882](https://github.com/getsphericon/sphericon/issues/882)) ([8a934bd](https://github.com/getsphericon/sphericon/commit/8a934bd4246e7776b9ac8527384550ce2edd0681))
* **automations:** exit and goal events end an Enrollment early ([#810](https://github.com/getsphericon/sphericon/issues/810)) ([bec4c0a](https://github.com/getsphericon/sphericon/commit/bec4c0a60f09a41adb5768c9d0dab8e32389d321))
* **automations:** results over a period on the Automation report, on /site and /api ([#753](https://github.com/getsphericon/sphericon/issues/753)) ([#820](https://github.com/getsphericon/sphericon/issues/820)) ([6e52de7](https://github.com/getsphericon/sphericon/commit/6e52de75c9091315e8a9c8650f18fd4b25c9fb2e))
* **automations:** steps render the triggering Event's properties as `event` ([#814](https://github.com/getsphericon/sphericon/issues/814)) ([41e50fe](https://github.com/getsphericon/sphericon/commit/41e50fe0b6709da597a3ff5d6418018b60131295))
* **condition:** count an event leaf at least or at most N times within a window ([#798](https://github.com/getsphericon/sphericon/issues/798)) ([7568bf6](https://github.com/getsphericon/sphericon/commit/7568bf6bafe329718877939d5620b82c45483495))
* **condition:** number and date leaves for typed Custom fields and trigger properties ([#823](https://github.com/getsphericon/sphericon/issues/823)) ([ad6ce3e](https://github.com/getsphericon/sphericon/commit/ad6ce3edf1673bff3232f77058724863caa02823))
* **config:** an Instance runs in one Mode, selfhosted or saas ([#687](https://github.com/getsphericon/sphericon/issues/687)) ([4bdf80f](https://github.com/getsphericon/sphericon/commit/4bdf80f0d4ded5b2c37183173f54eeb7998db50f))
* **console:** validate console forms against the generated staff schemas ([#850](https://github.com/getsphericon/sphericon/issues/850)) ([6c6f5f6](https://github.com/getsphericon/sphericon/commit/6c6f5f61e5eabb28ef42fe765656ad60433d7ce7))
* **contacts:** show an initials avatar next to every contact ([df06aa6](https://github.com/getsphericon/sphericon/commit/df06aa63e6d95fa4792858b589e80780aba8f7b4))
* **contacts:** show the country flag next to a country custom field ([9593d9a](https://github.com/getsphericon/sphericon/commit/9593d9a14ff8abf9d04b09293d17b24dd8bdc735))
* declare access per operation in the contract and enforce it once (ADR 0061) ([#776](https://github.com/getsphericon/sphericon/issues/776)) ([aaa0d18](https://github.com/getsphericon/sphericon/commit/aaa0d181d4af76fa597c0008e49955021c41e274))
* **dev:** fill the Brightpath demo with every state and fix db:seed on a running stack ([#693](https://github.com/getsphericon/sphericon/issues/693)) ([4e37a46](https://github.com/getsphericon/sphericon/commit/4e37a469a47b9723b4e21e4be3232764ecfd8ae6))
* **dev:** open the running stack at a public URL through a Cloudflare Quick Tunnel ([b17a233](https://github.com/getsphericon/sphericon/commit/b17a2335969e0e001f729184cc7068d30c6fea59))
* **dev:** start on a database that is behind and run its migrations from the error page ([5fb27af](https://github.com/getsphericon/sphericon/commit/5fb27afbcdc8ab7dbd243ec9df3499fdf8805178))
* **dev:** two-factor sign-in for Brightpath's admin and member, course links on brightpath.localhost ([#712](https://github.com/getsphericon/sphericon/issues/712)) ([430fdc8](https://github.com/getsphericon/sphericon/commit/430fdc8489da4edb605740026dbb221881d622bf))
* **dev:** verify Sending domains in dev through a CoreDNS daemon ([#692](https://github.com/getsphericon/sphericon/issues/692)) ([cdd1e95](https://github.com/getsphericon/sphericon/commit/cdd1e950dd7f76cc2009457bd23cb90a80f354ae))
* **events:** show System events by name instead of their action ([#903](https://github.com/getsphericon/sphericon/issues/903)) ([38a4622](https://github.com/getsphericon/sphericon/commit/38a46220574060feb4cf024eeef429cb2be180c8))
* every access decision is a Permission, no code compares a Role (ADR 0061) ([#780](https://github.com/getsphericon/sphericon/issues/780)) ([82b25fe](https://github.com/getsphericon/sphericon/commit/82b25fee4e9109e840192aaabeed99256ebf3214))
* **frontend:** invalidate a mutation's own resource by its operation tag ([59294d1](https://github.com/getsphericon/sphericon/commit/59294d1b1520826991728dc4ee9a89c7aee7b2e0)), closes [#829](https://github.com/getsphericon/sphericon/issues/829)
* **integrations:** edit preset params in a form generated from the schema ([#883](https://github.com/getsphericon/sphericon/issues/883)) ([5590e3a](https://github.com/getsphericon/sphericon/commit/5590e3acde0b889de77ef33c2ec9cf77a571e58c))
* **integrations:** read the SES account's enforcement status and sandbox with its quota (ADR 0067) ([#821](https://github.com/getsphericon/sphericon/issues/821)) ([b98f38e](https://github.com/getsphericon/sphericon/commit/b98f38edb4b95131bc5403ce8fe329b2482dd381))
* **integrations:** set another provider as the workspace default from its row ([d0dcd21](https://github.com/getsphericon/sphericon/commit/d0dcd219a9fdc7493558d676afc0a7786a47b005))
* **integrations:** show and restore archived providers ([#880](https://github.com/getsphericon/sphericon/issues/880)) ([056fa32](https://github.com/getsphericon/sphericon/commit/056fa327966559d304212529ec8d27a450ce1cf5))
* **integrations:** the Provider limit, required for an SES-compatible service (ADR 0067) ([#815](https://github.com/getsphericon/sphericon/issues/815)) ([d1de8e3](https://github.com/getsphericon/sphericon/commit/d1de8e3ef1c698b0ebc99456ac8cf8f13b28f220))
* **jobs:** LISTEN for river jobs through a pgx listener, stop softly ([#860](https://github.com/getsphericon/sphericon/issues/860)) ([901da6d](https://github.com/getsphericon/sphericon/commit/901da6d1759778ed7ce95e0b76a339201581e5e1))
* **mcp:** ship a set-up-sending-domain prompt ([#682](https://github.com/getsphericon/sphericon/issues/682)) ([eb3166f](https://github.com/getsphericon/sphericon/commit/eb3166fcd3de2a294dc0683ce3ef3269c594a9c7))
* **oauth:** let a connector manage Sending domains and Integrations behind a second opt-in ([#686](https://github.com/getsphericon/sphericon/issues/686)) ([ead9f3e](https://github.com/getsphericon/sphericon/commit/ead9f3e918dabd3411751be3b47a4143027249d1))
* **orders:** an Order Completed names its currency, and the first one sets the Reporting currency ([#896](https://github.com/getsphericon/sphericon/issues/896)) ([036f905](https://github.com/getsphericon/sphericon/commit/036f90545f44975b59a75eebaa974c0aa550c6e5))
* **revenue:** order Events, attribution and revenue reports (ADR 0064) ([#727](https://github.com/getsphericon/sphericon/issues/727)) ([fb34db7](https://github.com/getsphericon/sphericon/commit/fb34db7d999ed54e0d98ab491c14c5d441b83735))
* **sendcap:** a per-Workspace Send cap a Staff member sets on a saas Instance ([#871](https://github.com/getsphericon/sphericon/issues/871)) ([ccfc913](https://github.com/getsphericon/sphericon/commit/ccfc91391c6a40dbcf00c973b0d11c158f3b5fc0))
* **sending-domains:** show and restore archived domains ([#881](https://github.com/getsphericon/sphericon/issues/881)) ([f90d841](https://github.com/getsphericon/sphericon/commit/f90d841fbab2f4f0c7a0b9dfd5eac9b1c2bb5d49))
* **setup:** simplify the Setup checklist to four steps ([#680](https://github.com/getsphericon/sphericon/issues/680)) ([80feba0](https://github.com/getsphericon/sphericon/commit/80feba0f15b2f3e147c7f5a5a4e4dd8688dc2dec)), closes [#673](https://github.com/getsphericon/sphericon/issues/673)
* sign-up items, Consent records and a Staff console on every Instance ([#754](https://github.com/getsphericon/sphericon/issues/754)) ([7c9d4df](https://github.com/getsphericon/sphericon/commit/7c9d4dfd4b459fb912d9da4c7c2ac8eed0ba5d0b))
* **staff:** serve the job queue UI to Staff members and keep job logs ([#861](https://github.com/getsphericon/sphericon/issues/861)) ([fc72716](https://github.com/getsphericon/sphericon/commit/fc727162af3c261198f16af47bb7adadd5751f01))
* **staff:** the Instance settings move to the Staff console ([#799](https://github.com/getsphericon/sphericon/issues/799)) ([5bf0680](https://github.com/getsphericon/sphericon/commit/5bf0680ad052f0bb46c87351c2a8ed2e770537b6)), closes [#697](https://github.com/getsphericon/sphericon/issues/697)
* **tracking:** a Tracking domain is the Workspace's own, not a Sending domain's (ADR 0071) ([#904](https://github.com/getsphericon/sphericon/issues/904)) ([f56ec3d](https://github.com/getsphericon/sphericon/commit/f56ec3dbb4f229e81d3abb4e31f1b8af5dd746d6))
* **typespec:** contract extensions are declared with typed decorators ([#825](https://github.com/getsphericon/sphericon/issues/825)) ([12d8e56](https://github.com/getsphericon/sphericon/commit/12d8e565f0c53b1dbb279f893d9a166adcd43fea))
* **web:** a broadcast chooses the Integration it sends through ([#720](https://github.com/getsphericon/sphericon/issues/720)) ([c443725](https://github.com/getsphericon/sphericon/commit/c44372563bf255fb0db7e394bae19a4a88b0facd))
* **web:** a calmer error page whose commands copy with a click ([b65f42b](https://github.com/getsphericon/sphericon/commit/b65f42b5a8515c4c7c93b1c8dc1dc5a73217a005))
* **web:** add a favicon based on the Sphericon mark ([#678](https://github.com/getsphericon/sphericon/issues/678)) ([0d36500](https://github.com/getsphericon/sphericon/commit/0d365001997dc6e0e54bc8e0da8116330caa6f84))
* **web:** archived records are a list view, and domains and providers are removed ([6ea4361](https://github.com/getsphericon/sphericon/commit/6ea4361fab17e0711d4d53a23f6b527623bc1c5d))
* **web:** hide the controls whose Permission the Role lacks (ADR 0061) ([#777](https://github.com/getsphericon/sphericon/issues/777)) ([54cd392](https://github.com/getsphericon/sphericon/commit/54cd3923ca58975fb708c679d300f4875b6eb05a))
* **web:** move the Suppression list to its own menu item ([#681](https://github.com/getsphericon/sphericon/issues/681)) ([0111115](https://github.com/getsphericon/sphericon/commit/0111115384749ae776f0e8bc045213ec845f965a))
* **web:** overview lists broadcasts and automations together under Recently created ([5567605](https://github.com/getsphericon/sphericon/commit/5567605b2c41368ffa5351370bf0796fb1bc3173))
* **web:** overview shows create shortcuts and recent broadcasts once setup is done ([e037ca3](https://github.com/getsphericon/sphericon/commit/e037ca378e37231d0081e587c26418e2c739a388))
* **web:** report browser errors to Sentry when SPHERICON_SENTRY_DSN is set ([#670](https://github.com/getsphericon/sphericon/issues/670)) ([a2b9709](https://github.com/getsphericon/sphericon/commit/a2b9709627f96dd929c85117134105c66e98e7bd))
* **web:** show Sending domain DNS records in a modal with a prompt to hand to an agent ([#674](https://github.com/getsphericon/sphericon/issues/674)) ([4a68dba](https://github.com/getsphericon/sphericon/commit/4a68dba9ea6d6f22c252213d51d4877d319462bf)), closes [#672](https://github.com/getsphericon/sphericon/issues/672)
* **web:** show what Sphericon is beside the sign-in, sign-up and password recovery forms ([#685](https://github.com/getsphericon/sphericon/issues/685)) ([bf9099d](https://github.com/getsphericon/sphericon/commit/bf9099d8e276b9c5e3c69bdc39731b6a64f76895))


### Bug Fixes

* **account:** the Security page is as wide as the Profile page ([2525466](https://github.com/getsphericon/sphericon/commit/25254664024077f72602d0e3cb33ada271e71bc3))
* **automations:** archiving deactivates an Automation, and Automation and Rule share one draft lifecycle ([#800](https://github.com/getsphericon/sphericon/issues/800)) ([e6657c6](https://github.com/getsphericon/sphericon/commit/e6657c6f51712c99014b2c3b289d6e23860ddef9)), closes [#785](https://github.com/getsphericon/sphericon/issues/785)
* **broadcasts:** one recipient job per recipient when a plan is retried ([560744a](https://github.com/getsphericon/sphericon/commit/560744aca15c115546e2328d17ca8c63a4dfb6e5)), closes [#842](https://github.com/getsphericon/sphericon/issues/842)
* **config:** keep the secret variables in one list so doctor redacts every one ([0a101f9](https://github.com/getsphericon/sphericon/commit/0a101f938428567a7c55a505ee4c123c2ae04a7c)), closes [#832](https://github.com/getsphericon/sphericon/issues/832)
* **dev:** db:generate applies migrations with goose before it diffs ([#789](https://github.com/getsphericon/sphericon/issues/789)) ([45a0980](https://github.com/getsphericon/sphericon/commit/45a09801b870da87a451cc945059d3651370a37c))
* **emaildoc:** start a runtime without the code block's languages and register them before one ([#747](https://github.com/getsphericon/sphericon/issues/747)) ([ba13751](https://github.com/getsphericon/sphericon/commit/ba1375133af888df347f8d622bba63e5f7c42b9c))
* **events:** preallocate the handlers and say why the dead-letter tests write rows ([6b64026](https://github.com/getsphericon/sphericon/commit/6b64026fe8cadc3254aa5db45448b18f5925dddb))
* **events:** retry a failing domain-event handler, then dead-letter the event ([2c4fa96](https://github.com/getsphericon/sphericon/commit/2c4fa965179a4b5972d5f78c2d94049f3c745dbb)), closes [#845](https://github.com/getsphericon/sphericon/issues/845)
* every operation with input declares its 400, the frontend suite runs on pull requests, and the nightly is green ([#873](https://github.com/getsphericon/sphericon/issues/873)) ([f49aa75](https://github.com/getsphericon/sphericon/commit/f49aa753cb4a3ff0b37f31a98132af7878f203b5))
* **identities:** linking a sign-in provider keeps the other sessions signed in ([745b765](https://github.com/getsphericon/sphericon/commit/745b765ab55c743c673baef23e595d1ba072ea23))
* **identities:** the disconnect button of a linked provider is neutral, not red ([03eb6cc](https://github.com/getsphericon/sphericon/commit/03eb6cc60d63284adf6d7cc961d8d99668e4eda3))
* **integrations:** one alert per Provider limit warning, none for a Test inbox ([#908](https://github.com/getsphericon/sphericon/issues/908)) ([793308a](https://github.com/getsphericon/sphericon/commit/793308a6005a682c83a6bb33ff313d20c56e159f))
* **integrations:** the Preset params form checks formats as well ([#902](https://github.com/getsphericon/sphericon/issues/902)) ([e8bc575](https://github.com/getsphericon/sphericon/commit/e8bc5757e5e9fca755d04c0e63ca790e6908565e))
* **jobs:** queue the listener test's welcome through mailjobs ([4f5980d](https://github.com/getsphericon/sphericon/commit/4f5980d7651bf180996936273e810833d0a2fbae))
* **mcp:** a tool call ends with the /mcp request that runs it ([#766](https://github.com/getsphericon/sphericon/issues/766)) ([8b644c6](https://github.com/getsphericon/sphericon/commit/8b644c6aada5b99488ace201cdb206f351739217))
* **migrations:** drop tracking_host_changes.domain before logging the removed hosts ([#906](https://github.com/getsphericon/sphericon/issues/906)) ([16683c1](https://github.com/getsphericon/sphericon/commit/16683c1316e2c210ba8a348c4641a9f6c2c8d403))
* **outbound:** a Broadcast Test send goes through the Broadcast's own Integration ([#694](https://github.com/getsphericon/sphericon/issues/694)) ([d2d4fd5](https://github.com/getsphericon/sphericon/commit/d2d4fd540cc27fe98339c754c74adebbb9fed0e1))
* **passkeys:** adding a passkey keeps the other sessions signed in ([a86d7b6](https://github.com/getsphericon/sphericon/commit/a86d7b62d506486aacd14e4952adfd8e1e7e2dab))
* **profile:** sign out everywhere is a neutral action, not a red one ([3783dbf](https://github.com/getsphericon/sphericon/commit/3783dbf83c51853b3a6e191f930ae19e65080545))
* **profile:** sign out everywhere is yellow, a caution rather than a deletion ([3be0fc7](https://github.com/getsphericon/sphericon/commit/3be0fc79be4ff9f1f57cc9f5ea30c79812cf0c4d))
* **settings:** toast after a member's Role changes and show a loader while it saves ([fb2d323](https://github.com/getsphericon/sphericon/commit/fb2d323074a3ef04216e2459c487f1110fb964db))
* **settings:** toast after removing a member, revoking an invitation and resetting 2FA ([60ce26e](https://github.com/getsphericon/sphericon/commit/60ce26ef23914ec1cb917d355618dc9945c68118))
* **testhelper:** report the close of a test job's pool instead of dropping it ([3e0abe0](https://github.com/getsphericon/sphericon/commit/3e0abe024b6f57172961b07cab2d94aa43ef4217))
* **web:** a navbar link on a phone swaps the page without the cross-fade ([4742991](https://github.com/getsphericon/sphericon/commit/47429910d6a2774fe4ac317f5c8d71db4befb7bc))
* **web:** move sign-in providers below the register form ([#679](https://github.com/getsphericon/sphericon/issues/679)) ([d103579](https://github.com/getsphericon/sphericon/commit/d10357961424c7deb95ad4278a811a3c43f7fd7f))
* **web:** move the sign-in providers below the form on the register page ([d103579](https://github.com/getsphericon/sphericon/commit/d10357961424c7deb95ad4278a811a3c43f7fd7f))
* **web:** quieter Setup checklist, Sending domain modal in tabs, Verify waits for the check ([d0c3ba8](https://github.com/getsphericon/sphericon/commit/d0c3ba8dc3b3eb1e53710b453f3228014e224307))
* **web:** setup checklist actions sit on the right and drop below when narrow ([49967c4](https://github.com/getsphericon/sphericon/commit/49967c4a80c7d0916e4aabfedb40dae70da893cd))
* **web:** setup checklist rows share one container grid so actions wrap together ([08340f3](https://github.com/getsphericon/sphericon/commit/08340f3a400591d94d89e69b9780597ad410dd7c))
* **web:** sign-in and sign-up pages no longer scroll on a phone ([2ea14fd](https://github.com/getsphericon/sphericon/commit/2ea14fdab42c17b55b40cb905f43a3e7d5a20e19))
* **web:** switching an archive view keeps the table and cross-fades its rows ([ebb50f0](https://github.com/getsphericon/sphericon/commit/ebb50f009ef0c4af2a43885e7482b8b8baab2525))
* **web:** the Workspace switcher fits a phone header and its list no longer clips ([831f20f](https://github.com/getsphericon/sphericon/commit/831f20fc0fdbe5f15b2d9f852716931c9e785c76))

## [0.3.5](https://github.com/getsphericon/sphericon/compare/v0.3.4...v0.3.5) (2026-10-27)


### Features

* **automations:** let an Automation name the Integration its email steps send through ([#667](https://github.com/getsphericon/sphericon/issues/667)) ([916c67c](https://github.com/getsphericon/sphericon/commit/916c67c791a3a8db148c9034e26ecbb3438e217f)), closes [#662](https://github.com/getsphericon/sphericon/issues/662)
* **email:** add SPHERICON_SYSTEM_EMAIL_REPLY_TO and accept a display name in SPHERICON_SYSTEM_EMAIL_FROM ([#666](https://github.com/getsphericon/sphericon/issues/666)) ([784ca4d](https://github.com/getsphericon/sphericon/commit/784ca4dd3e76e9a13a9a09fcb68b303a57f7f412))


### Bug Fixes

* **broadcasts:** send and pace a Broadcast through its own Integration ([e548e60](https://github.com/getsphericon/sphericon/commit/e548e6056e31e9a9b0293b23f443c1036e0c9d23)), closes [#661](https://github.com/getsphericon/sphericon/issues/661)
* make the test suite independent of the machine's time zone and personal .env ([#658](https://github.com/getsphericon/sphericon/issues/658)) ([16ba5b4](https://github.com/getsphericon/sphericon/commit/16ba5b419f8b8b77d4fc8781e7fca4d43e448829))


### Performance Improvements

* **credentials:** hash token secrets with SHA-256 and parse tokens by their exact length ([#660](https://github.com/getsphericon/sphericon/issues/660)) ([4834d78](https://github.com/getsphericon/sphericon/commit/4834d785c09853bd55bb349a2c77eab2f7c723d6))

## [0.3.4](https://github.com/getsphericon/sphericon/compare/v0.3.3...v0.3.4) (2026-10-26)


### Bug Fixes

* start the document renderer's runtime at boot, not on the first save ([6109285](https://github.com/getsphericon/sphericon/commit/61092851c1ed3eaf0ad49b063a84501a3db49eb9))

## [0.3.3](https://github.com/getsphericon/sphericon/compare/v0.3.2...v0.3.3) (2026-10-26)


### Features

* log every request and the reason a request was refused ([f133d3f](https://github.com/getsphericon/sphericon/commit/f133d3fb4d08e2138c6ad29a03e48a3ea9f82c8f))
* pick a Broadcast's From from the verified Sending domains ([40e6e99](https://github.com/getsphericon/sphericon/commit/40e6e990594b4480aec69e16ec55afc92840f1bf)), closes [#656](https://github.com/getsphericon/sphericon/issues/656)


### Bug Fixes

* center the create workspace page ([55d9c7f](https://github.com/getsphericon/sphericon/commit/55d9c7ffd151a0878bfa80c0be34c4702135335c))
* commit the mise lock sidecar of scip-typescript ([ceb1a49](https://github.com/getsphericon/sphericon/commit/ceb1a496bb969ca1444972cb9734cc22aec9c301))

## [0.3.2](https://github.com/getsphericon/sphericon/compare/v0.3.1...v0.3.2) (2026-10-25)


### Features

* a Test inbox provider keeps what it is sent as Captured messages ([d9dab34](https://github.com/getsphericon/sphericon/commit/d9dab3476ed844823934e52fb64b8a488745166a))
* add a Setup checklist step that creates a Test inbox without making it the default ([0f6c664](https://github.com/getsphericon/sphericon/commit/0f6c6643bd9fe1e59f3eea5e4c5835f59da1005b))
* bound the Test inbox to 200 a day, 7 days and 200 kept ([64116e4](https://github.com/getsphericon/sphericon/commit/64116e42fbb9812fcc63f34deafcdafd887b2896))
* capture a Test send through a Test inbox instead of the System sender ([44dfd52](https://github.com/getsphericon/sphericon/commit/44dfd528f93a1027cdeb2b0ca265b332009df51f))
* **erasure:** erase Captured messages with a Contact's address ([b054673](https://github.com/getsphericon/sphericon/commit/b054673d17dcd3c4ebc7b8782357adad7a45addf))
* read, delete one and delete all Captured messages in the site and external API ([0450067](https://github.com/getsphericon/sphericon/commit/04500678062efd7a40b56d13446db2db328e377b))
* say "captured, not delivered" on every report of a send through a Test inbox ([e74a5c8](https://github.com/getsphericon/sphericon/commit/e74a5c85b9de2542a48295b7d20f55e1e485cd1c))
* serve the first-party tracker on a Tracking domain and pick its host in the snippet ([df6838b](https://github.com/getsphericon/sphericon/commit/df6838b5ad867515d8033db7d245fb01fb5a4976))
* **ui:** build the tracking snippet on the tracker's own origin and link the SDKs and the guide ([d3d8c6e](https://github.com/getsphericon/sphericon/commit/d3d8c6e85275fbb1f642a4ba63d91fbea07f4607))
* **ui:** choose the Test inbox and read its Captured messages ([77269aa](https://github.com/getsphericon/sphericon/commit/77269aa0a38e0c83972f179a92e240d548d5028c))
* **ui:** move the analytics reports menu into the workspace sidebar ([e221298](https://github.com/getsphericon/sphericon/commit/e2212984adc026e2848fb26a151507f9477085f8))


### Bug Fixes

* **ee:** replace the embedded license public key ([8f36872](https://github.com/getsphericon/sphericon/commit/8f36872adca055772c5e3b4708223ff648318378))
* follow the confirmation hook rename, type the Origin surface, cover the Test inbox scenarios ([1ab4d7a](https://github.com/getsphericon/sphericon/commit/1ab4d7a2bc0e7916ae313dff6480ba3dc942a8d7))
* **i18n:** add a Passkey instead of register it, and sign up in the verify error ([701ac22](https://github.com/getsphericon/sphericon/commit/701ac2259551d55a49b995d22742d33c9661778e))
* **ui:** ask for the Sending domains with the page size the other forms use ([85aea49](https://github.com/getsphericon/sphericon/commit/85aea49910e720625a54271d489afd183546220d))
* **ui:** fit the header, setup checklist and analytics layout on phones ([e05d165](https://github.com/getsphericon/sphericon/commit/e05d165182bf45a94a3b19bd2d3345fe77474295))
* **ui:** hide the telemetry notice while the environment keeps the report off ([490d43e](https://github.com/getsphericon/sphericon/commit/490d43e061051703e2aa8b95cbacaa42ce7bb44d))
* **ui:** keep analytics subpages within the screen on phones ([b16399e](https://github.com/getsphericon/sphericon/commit/b16399eef9c2b0708fbf2ec7942d5a4c4958a08c))
* **ui:** keep the header and the automation builder bar usable on narrow screens ([b780df8](https://github.com/getsphericon/sphericon/commit/b780df80901c5d9babe5d75766faf49ec27ed9dc))
* **ui:** say sign up instead of register in the sign-up page and the mail ([6e8d8bb](https://github.com/getsphericon/sphericon/commit/6e8d8bb58d91586492464606a779a34a3174f079))
* **ui:** send a signed-in visitor from the login and register pages to the app ([a0a4f40](https://github.com/getsphericon/sphericon/commit/a0a4f4068c321747507ff4b1c837ace4c4057f3d))

## [0.3.1](https://github.com/getsphericon/sphericon/compare/v0.3.0...v0.3.1) (2026-10-25)


### Bug Fixes

* centre the sign-in and standalone pages inside a Container ([db9166e](https://github.com/getsphericon/sphericon/commit/db9166e3106d5fcf38da1474fb14b99deb35e793))
* **dev:** drop a worktree's databases when it is removed ([ed8aa9b](https://github.com/getsphericon/sphericon/commit/ed8aa9bc9750dc3ffdf9dd8fddbf593db798402e))
* **dev:** make db:up show why the db daemon did not start ([7626540](https://github.com/getsphericon/sphericon/commit/762654049272fed7f8db01a98e35247dc15460ba))
* **dev:** run the daily worktrees:gc only from the primary checkout ([3604d72](https://github.com/getsphericon/sphericon/commit/3604d723797131fb3205a1ac6dcc5404c6cea960))

## [0.3.0](https://github.com/getsphericon/sphericon/compare/v0.2.0...v0.3.0) (2026-10-24)


### ⚠ BREAKING CHANGES

* the site API's Broadcast resource no longer has `bodyText`, and the `broadcasts` table no longer has `body_text`.
* the Segment field `definition` is `condition` in the site and external APIs, and the segments.definition column is renamed to segments.condition. Upgrade: run the migrations (sphericon migrate); API clients send and read `condition` instead of `definition` on Segments and their preview.

### Features

* a Rule applies and removes Tags on an Event (tracer bullet, ADR 0054) ([73991d1](https://github.com/getsphericon/sphericon/commit/73991d11345e052c9e5899ff7ef638bb8379393d))
* a Rule can call a URL when it fires (HTTP Action, ADR 0054) ([f7d954c](https://github.com/getsphericon/sphericon/commit/f7d954c0001f2177e43c93aeaa3061608d226daa))
* a Rule enrolls a Contact in an Automation (ADR 0054) ([efe6bb3](https://github.com/getsphericon/sphericon/commit/efe6bb34514c19030e550bc91ddc6507816bba96))
* add the Rules UI with list, editor and Firings ([#598](https://github.com/getsphericon/sphericon/issues/598)) ([40ff2e0](https://github.com/getsphericon/sphericon/commit/40ff2e0ece984d92226365b54f3f2b04356afc92))
* add Workflows and Website groups to the sidebar ([d8733cf](https://github.com/getsphericon/sphericon/commit/d8733cf5910d5666e12d816b813b9e5feffab1e9))
* an opt-in Sign-up survey asked once after first sign-in (ADR 0055) ([2f5c5d5](https://github.com/getsphericon/sphericon/commit/2f5c5d504dcdb2c55554efde25aa6ce1845f5a90)), closes [#562](https://github.com/getsphericon/sphericon/issues/562) [#563](https://github.com/getsphericon/sphericon/issues/563) [#564](https://github.com/getsphericon/sphericon/issues/564) [#565](https://github.com/getsphericon/sphericon/issues/565) [#566](https://github.com/getsphericon/sphericon/issues/566) [#567](https://github.com/getsphericon/sphericon/issues/567)
* analytics section with reports in the Workspace time zone ([#627](https://github.com/getsphericon/sphericon/issues/627)) ([140bf60](https://github.com/getsphericon/sphericon/commit/140bf60356f2a6c61f086777e5ff6ffdef3e02d6))
* archive, purge and limit Rules; Firings expire with the analytical window (ADR 0054) ([4fbfbba](https://github.com/getsphericon/sphericon/commit/4fbfbba5a778382d5d85e697174797358fb63f94))
* archive, restore and delete Rules in the UI ([5441e81](https://github.com/getsphericon/sphericon/commit/5441e81ffb1e4cff75e4e198d5aa78fb63945b81))
* check passwords and account addresses beyond the contract ([#612](https://github.com/getsphericon/sphericon/issues/612)) ([72322d1](https://github.com/getsphericon/sphericon/commit/72322d1f05d9febcb84d2df5e32b7efa3240e0c4))
* derive the marketing text part from the final HTML of a send ([c2574c6](https://github.com/getsphericon/sphericon/commit/c2574c692cfe0c32cc5e42ceee7e522962307a54)), closes [#617](https://github.com/getsphericon/sphericon/issues/617)
* dev Workspace and live demo storefront on brightpath.localhost ([#609](https://github.com/getsphericon/sphericon/issues/609)) ([bd52b64](https://github.com/getsphericon/sphericon/commit/bd52b6409f97cd04dcf0a3209ac2273ccc50e003))
* dry-run a Rule on a stored Event ([#603](https://github.com/getsphericon/sphericon/issues/603)) ([6936510](https://github.com/getsphericon/sphericon/commit/6936510f490ff386cb20f6cfba837feb7f60ab73))
* move leaving a workspace into the danger zone and calm the buttons ([60ca144](https://github.com/getsphericon/sphericon/commit/60ca144cd7a4fc41ea351f72efd21bfb1a4ab431))
* rename API keys, compact Developers settings page ([c2e3fc9](https://github.com/getsphericon/sphericon/commit/c2e3fc90e4ce06b93f9a2d8a0cad6cea42c21d4b))
* show skeletons instead of spinners while pages and lists load ([8ec36d7](https://github.com/getsphericon/sphericon/commit/8ec36d70e34038a38594632c2084e728a6b70fdf))
* test the triggering Event's properties in a Rule's Condition ([#602](https://github.com/getsphericon/sphericon/issues/602)) ([244b50a](https://github.com/getsphericon/sphericon/commit/244b50a01860e596d0c5633c401acc075bbf3492))
* track every email event for every send (ADR 0053) ([5e61e1f](https://github.com/getsphericon/sphericon/commit/5e61e1f53d1bb234314f6efae03c51fc4837e3df))


### Bug Fixes

* a dry run says a machine Event never fires a Rule ([369061a](https://github.com/getsphericon/sphericon/commit/369061abe712f6c702f985da011f4dafa901ee9d))
* a machine open or click triggers no Rule ([30a10ae](https://github.com/getsphericon/sphericon/commit/30a10aee73393691cce6c408b72077b04da885d2))
* an HTTP step and a Rule's HTTP Action fail at once on a 4xx ([2a14157](https://github.com/getsphericon/sphericon/commit/2a14157abf9864f922c648d0e73d83f32c6c3bb7))
* archive a Rule in one guarded write that locks the row ([c7cff7c](https://github.com/getsphericon/sphericon/commit/c7cff7c022e5bd04a253311066481fdc21464da4))
* call the sign-in test helpers with their current signatures in the survey tests ([75b089a](https://github.com/getsphericon/sphericon/commit/75b089abd2b4a26109618c83e1c22fd7fb08c799))
* derive the text part in one step and hide more than display:none ([8bbabef](https://github.com/getsphericon/sphericon/commit/8bbabefbe98baf67ee47cefdc976432881761a02)), closes [#616](https://github.com/getsphericon/sphericon/issues/616) [#617](https://github.com/getsphericon/sphericon/issues/617) [#618](https://github.com/getsphericon/sphericon/issues/618)
* enroll a Contact and queue its first step in one transaction ([ddb86ec](https://github.com/getsphericon/sphericon/commit/ddb86ece45d39deede3f58bd0c7a7509f9115ee3))
* have the server quote database names and quote them in the log ([7a39dfa](https://github.com/getsphericon/sphericon/commit/7a39dfa06b58f23ef21c0d559b46e9d132d1ed31))
* index rule_firings by workspace_id for its foreign key ([b5dfc47](https://github.com/getsphericon/sphericon/commit/b5dfc475c8cf733a991d5499101d119c24196a53)), closes [#595](https://github.com/getsphericon/sphericon/issues/595) [#596](https://github.com/getsphericon/sphericon/issues/596) [#597](https://github.com/getsphericon/sphericon/issues/597) [#598](https://github.com/getsphericon/sphericon/issues/598) [#599](https://github.com/getsphericon/sphericon/issues/599) [#600](https://github.com/getsphericon/sphericon/issues/600) [#601](https://github.com/getsphericon/sphericon/issues/601) [#602](https://github.com/getsphericon/sphericon/issues/602) [#603](https://github.com/getsphericon/sphericon/issues/603)
* keep the backfill flag on the stored Event so a dry run tells history from a live Event ([68dab50](https://github.com/getsphericon/sphericon/commit/68dab507c5d8d9b99b69b364d8652778b2a52d17))
* log a Rule's stored actions or results that cannot be decoded ([7422502](https://github.com/getsphericon/sphericon/commit/7422502b000299d203b8672e1df8490571c68532))
* log the lost lookups and the table that cannot load, keep the text part's visible body ([e29e05c](https://github.com/getsphericon/sphericon/commit/e29e05c4f222c47564ac60fa128864365e127f49))
* refuse a blank Broadcast name in the Broadcasts module on every surface ([9e8b48f](https://github.com/getsphericon/sphericon/commit/9e8b48fa933e9c6e6c0e9c8bbd7745b83510dbcc)), closes [#630](https://github.com/getsphericon/sphericon/issues/630)
* refuse a whitespace-only Template name in the Templates module ([8de1d95](https://github.com/getsphericon/sphericon/commit/8de1d951e7d4339008b1f1b650a2619667874932))
* refuse deleting an Automation that sent mail, and expose enroll_automation in /api ([e9c1d9e](https://github.com/getsphericon/sphericon/commit/e9c1d9ecb1b0aafd5a3c9a32c5bed2ad5187673f))
* retry a Rule's HTTP call as an HTTP step does, on a job of its own ([848b5e9](https://github.com/getsphericon/sphericon/commit/848b5e988264003ff68f3abc0b4e632db15acc3a))
* retry a send whose tracking host lookup failed instead of failing it for good ([2010e5b](https://github.com/getsphericon/sphericon/commit/2010e5b8fee067ca81b19ee15c33317e8c61f25c))
* return to the requested page after login, neutral Sign out, document the white editor sheet ([e65422a](https://github.com/getsphericon/sphericon/commit/e65422af3c4cbb3c712a04ccb38f904edb43aa7d))
* show a Rule the same bounded Event properties in Condition, Firing and dry run ([165a0e6](https://github.com/getsphericon/sphericon/commit/165a0e6efd7afc91394e51c0ec6fe942b84b7f6d))
* stop full page reloads in the dev server and lint for the causes ([a2df4f8](https://github.com/getsphericon/sphericon/commit/a2df4f8c8b728d6b85e62ce32adbb2a21146c21b))
* stop swallowing errors, and log the ones nobody can act on ([1fae86f](https://github.com/getsphericon/sphericon/commit/1fae86feae8d137a47fbd3e6c89f8e1130ef5b4b))
* store account addresses lower-cased so one address is one account ([#613](https://github.com/getsphericon/sphericon/issues/613)) ([e1acc6d](https://github.com/getsphericon/sphericon/commit/e1acc6d1b961c18d7cda64247672e58b28d2e808))
* **testhelper:** take pgtestdb's template lock on the shared test database ([c93bd69](https://github.com/getsphericon/sphericon/commit/c93bd69acf6d96a41e0d10de6e012fa67314651f)), closes [#615](https://github.com/getsphericon/sphericon/issues/615)


### Code Refactoring

* drop the Broadcast body_text column and the bodyText field of its resource ([e02122a](https://github.com/getsphericon/sphericon/commit/e02122a44f9b73ee52c1dc76951e10b6950bce4c))
* name the Segment's rule tree a shared Condition ([d564770](https://github.com/getsphericon/sphericon/commit/d564770bb8d71fe3ec91fc63010aec239ba3099e))

## [0.2.0](https://github.com/getsphericon/sphericon/compare/v0.1.2...v0.2.0) (2026-10-21)

### ⚠ BREAKING CHANGES

- SPHERICON_PGX_MAX_CONNS is renamed SPHERICON_RIVER_DB_MAX_CONNS.
- **workspaces:** the Workspace field deletedAt is discardedAt in the site, staff and service APIs. Upgrade: update any client of the Workspace delete, deleted-workspaces list or workspace feed that reads deletedAt to read discardedAt.
- workspaces.deleted_at is renamed discarded_at, and DELETE on a sent or failed Broadcast now answers 422 (archive it instead).
- **automations:** automations.definition is dropped; an Automation's steps live in automation_versions. Upgrade: run `sphericon migrate`; the drop refuses to run while an Automation has no version. Start the server and let the river job backfill_automation_versions finish (it logs "automation versions backfilled"), then migrate again. A development database can be rebuilt with `mise run db:reset`.
- enum columns get CHECK constraints and telemetry_enabled is NOT NULL (true)
- prefix-redundant indexes are dropped, unique indexes lead with workspace_id
- a Broadcast recipient no longer stores status, sent_at, error or outbound_message_id, and a Broadcast no longer stores recipients_total, sent_count, opened_count, clicked_count, unsubscribed_count, failed_count or skipped_count; the contact export's broadcast recipient drops status, error and sentAt (its outbound messages carry them) and gains unsubscribedAt. The migration refuses to run while a decided recipient has no outbound message. Upgrade: run `sphericon migrate`. If it stops with "broadcast_recipients decided without an outbound message", upgrade through the previous release first (it records one message per recipient) and migrate again; a development database can be rebuilt with `mise run db:reset`.
- create, rename and delete Workspaces; address them by id, not slug ([#426](https://github.com/getsphericon/sphericon/issues/426))
- the ru and es locales are gone; SPHERICON_APP_LOCALE accepts only en.
- the Workspace ingest key and the `/hooks/{key}/{provider}` URL are removed. Upgrade: open each Integration, copy its new hook URL (`hookUrl`) and paste it into the provider (for SES, the SNS subscription), replacing the old Workspace-wide URL.

### Features

- a Test send from the System sender while a Workspace has no verified Sending domain ([334c7de](https://github.com/getsphericon/sphericon/commit/334c7de500c7bffeb143807735b7111244835e3f))
- a User leaves a Workspace themself (adr 0050) ([808d75b](https://github.com/getsphericon/sphericon/commit/808d75b0961fbcf6b2c8eafad25c4e9f05f0aa68)), closes [#510](https://github.com/getsphericon/sphericon/issues/510)
- a User with a password deletes themself (adr 0050) ([42b9507](https://github.com/getsphericon/sphericon/commit/42b9507c688491982f2163343b276a41764ec517)), closes [#511](https://github.com/getsphericon/sphericon/issues/511)
- a User with no password deletes themself through a mailed link (adr 0050) ([08b6a92](https://github.com/getsphericon/sphericon/commit/08b6a92d9054d3d2fa57eaef4a88f8c5e30d4807)), closes [#512](https://github.com/getsphericon/sphericon/issues/512)
- **accounts:** invitation for a Verified User requires their session (ADR 0030) ([d51cbfd](https://github.com/getsphericon/sphericon/commit/d51cbfd46eea5d3af2b4fc89b3c72be97baa94b6))
- add `sphericon doctor` to check an Instance's configuration ([cf79b6e](https://github.com/getsphericon/sphericon/commit/cf79b6ebb0a794bb78dbd8de8ae21e58b374955c))
- add a title to every list page, with the add button beside it ([bc4465a](https://github.com/getsphericon/sphericon/commit/bc4465a0eb356186b52e9f54bce41c66e5de7d48))
- add a WAF web ACL on the hosted ALB with a rate rule for mcp and collect ([2e260d6](https://github.com/getsphericon/sphericon/commit/2e260d65ecc6e571a189ad54fceefaf27b67598d)), closes [#266](https://github.com/getsphericon/sphericon/issues/266)
- add CloudWatch alarms and an SNS notification topic for the hosted deployment ([7c3ed36](https://github.com/getsphericon/sphericon/commit/7c3ed36c4e1a5592353a628235f7c3e9ea123f17)), closes [#265](https://github.com/getsphericon/sphericon/issues/265)
- add the audit export stream at GET /api/export/audit ([47112c2](https://github.com/getsphericon/sphericon/commit/47112c2e75b0e0380134ad58433449e477b2deaf)), closes [#432](https://github.com/getsphericon/sphericon/issues/432)
- add the Contacts Export stream ([dcd0f79](https://github.com/getsphericon/sphericon/commit/dcd0f79befc73cc4fea8d79657d3537d783b37e2))
- add the Erasures Export stream ([832ecbd](https://github.com/getsphericon/sphericon/commit/832ecbd168862d3ed00aa56f874bf92c2fd4a3ab)), closes [#431](https://github.com/getsphericon/sphericon/issues/431)
- add the Events Export stream and the shared Export stream foundation ([bd34505](https://github.com/getsphericon/sphericon/commit/bd34505a6a18db618e99bc02614bc7b27e72e93c)), closes [#428](https://github.com/getsphericon/sphericon/issues/428)
- add the Outbound messages Export stream ([03a6bf0](https://github.com/getsphericon/sphericon/commit/03a6bf0ef5c8f9d69b35b73a153bae114187162c)), closes [#429](https://github.com/getsphericon/sphericon/issues/429)
- add through modals, move row actions into a menu and show Integrations on Sending ([1ef7e3e](https://github.com/getsphericon/sphericon/commit/1ef7e3eba4ab87746585a1111524a4a6b12e6ab1))
- an expired license key falls back to the core ([ddef31f](https://github.com/getsphericon/sphericon/commit/ddef31f57a7ff11321ea04b3b50ee2fc68639dbe)), closes [#320](https://github.com/getsphericon/sphericon/issues/320)
- animate page navigation with router view transitions ([8c5dc85](https://github.com/getsphericon/sphericon/commit/8c5dc853bb61493731a1fb66f0a7eee70fe64206))
- anonymous Telemetry report and Release check (ADR 0032) ([#349](https://github.com/getsphericon/sphericon/issues/349)) ([80e0070](https://github.com/getsphericon/sphericon/commit/80e007048203bc2b5d7a51969830275d37825501))
- answer 503 naming the migrate command while migrations are missing ([585785e](https://github.com/getsphericon/sphericon/commit/585785ecefe43443be9d6c4bc8ebf53b0fe44743))
- archive an Integration instead of deleting it (ADR 0049) ([b177fd9](https://github.com/getsphericon/sphericon/commit/b177fd98db018a5324f6107be8e9637bf7df64f4)), closes [#528](https://github.com/getsphericon/sphericon/issues/528)
- **auth:** add the require-email-verification switch for installs without mail ([0e79086](https://github.com/getsphericon/sphericon/commit/0e79086d8909af998ef6334fc18daa1d9a05a5c4))
- **auth:** no endpoint confirms that an address is taken ([#272](https://github.com/getsphericon/sphericon/issues/272)) ([955f923](https://github.com/getsphericon/sphericon/commit/955f9235a86449d02b65830d8932ffb6af5ef45f))
- author email in an embedded editor, the server builds the HTML (ADR 0040) ([#403](https://github.com/getsphericon/sphericon/issues/403)) ([e9f7caf](https://github.com/getsphericon/sphericon/commit/e9f7caff11ef1912dd86a86c20a406f255de6284))
- **auth:** password reset proves the address of an Unverified User (ADR 0030) ([cacc015](https://github.com/getsphericon/sphericon/commit/cacc015dc828336f9b76300307709c14a980e660))
- **auth:** register without a password, verify by link, gate login (ADR 0030) ([238c70c](https://github.com/getsphericon/sphericon/commit/238c70c27d66032e8646b8aff1b11d9b9ae63804))
- **auth:** resend the verification link from the login refusal (ADR 0030) ([0b28e8d](https://github.com/getsphericon/sphericon/commit/0b28e8deb00548199e6ee36d79cfb216c6fa6612))
- **automations:** archive and purge an Automation with the Discard mixin ([b55796d](https://github.com/getsphericon/sphericon/commit/b55796da6e1bd4f92030c81145b349a91b7a40e7)), closes [#493](https://github.com/getsphericon/sphericon/issues/493)
- **billing:** core metering, Live usage and Billing hold for the billing plane (ADR 0027) ([ed42025](https://github.com/getsphericon/sphericon/commit/ed4202557d33ef74279cac1356c7353de9ed2b85))
- **billing:** core metering, Live usage and Billing hold for the billing plane (ADR 0027) ([#257](https://github.com/getsphericon/sphericon/issues/257)) ([ed42025](https://github.com/getsphericon/sphericon/commit/ed4202557d33ef74279cac1356c7353de9ed2b85))
- carry the Registration browser through resend and widen its cookie to the auth prefix ([b1efd01](https://github.com/getsphericon/sphericon/commit/b1efd01de2cf680a984fbb5ed971b94098c99e65))
- close sign-up with SPHERICON_SIGNUP_OPEN and create the first Owner with user create-owner ([7102a29](https://github.com/getsphericon/sphericon/commit/7102a2939da1cd73d2c79da5d37ee6aef8cf5601))
- **collect:** bound batch size and field lengths in the /collect contract ([#261](https://github.com/getsphericon/sphericon/issues/261)) ([bc3e747](https://github.com/getsphericon/sphericon/commit/bc3e7474f2dedb9a2a9eabfc75e93c61a8ec717a)), closes [#94](https://github.com/getsphericon/sphericon/issues/94)
- copy the MCP address from a compact field and drop section borders in settings ([91862c4](https://github.com/getsphericon/sphericon/commit/91862c442fbff2c1fbf7143fad3bd8987a33bc53))
- create, rename and delete Workspaces; address them by id, not slug ([#426](https://github.com/getsphericon/sphericon/issues/426)) ([437c99b](https://github.com/getsphericon/sphericon/commit/437c99bf4aeba1d01bf000138dc1254a6b41dc79))
- cross-fade between pages with React ViewTransition ([019c47b](https://github.com/getsphericon/sphericon/commit/019c47ba5778036f0c94559740e47234d47b5071))
- describe each app section in the docs and link the docs from the header ([d411918](https://github.com/getsphericon/sphericon/commit/d41191821e9104a18d172c384d2190057b2499d6))
- dismiss the Setup checklist ([286e40a](https://github.com/getsphericon/sphericon/commit/286e40af18d74fb7db3ac5a9c11074ea593bf1cc))
- erase an address from the Privacy requests section ([3b6a441](https://github.com/getsphericon/sphericon/commit/3b6a441685f764d6b773ad46bde310ebf7328919)), closes [#422](https://github.com/getsphericon/sphericon/issues/422)
- explain a dead backend in the dev edge instead of an empty 502 ([e1bcc33](https://github.com/getsphericon/sphericon/commit/e1bcc33726087d6507080e98e1069b0f7b85fe54))
- export a found contact from Privacy requests ([0ce4f2a](https://github.com/getsphericon/sphericon/commit/0ce4f2ac76f8354b532c642c0b773842399a0eb1)), closes [#423](https://github.com/getsphericon/sphericon/issues/423)
- inbound provider callbacks are Presets on an Integration ([#362](https://github.com/getsphericon/sphericon/issues/362)) ([1f4e17d](https://github.com/getsphericon/sphericon/commit/1f4e17d44550c231f3ffae3a70e7ea6671630954))
- **infra:** harden the production database: Multi-AZ, deletion protection, backups ([23619ab](https://github.com/getsphericon/sphericon/commit/23619abbfd880ce49eff13434186a82c34c16e18)), closes [#264](https://github.com/getsphericon/sphericon/issues/264)
- **infra:** verify test recipients in SES while the account is in the sandbox ([6595d9e](https://github.com/getsphericon/sphericon/commit/6595d9e4e4d1d505044864d950e3142e82a6567a))
- invite members from a modal, put row actions in a menu and fix nav highlighting ([a3b2746](https://github.com/getsphericon/sphericon/commit/a3b27461994cac669b4abb658e19f377e115de4b))
- **jobs:** prune stale Unverified Users ([#258](https://github.com/getsphericon/sphericon/issues/258)) ([aed31c2](https://github.com/getsphericon/sphericon/commit/aed31c24e0bedc7690132d3f35c1cae00f3fbe69))
- join domains, members join a Workspace by proven email domain ([#514](https://github.com/getsphericon/sphericon/issues/514)) ([d6e005c](https://github.com/getsphericon/sphericon/commit/d6e005c2e1ace17d2a630f714f1c564a41d7b487))
- **licensekey:** issue license keys as Ed25519-signed JWTs ([#343](https://github.com/getsphericon/sphericon/issues/343)) ([d3ced79](https://github.com/getsphericon/sphericon/commit/d3ced793bfff748ef61a1035b8027dd9f59da0bc)), closes [#336](https://github.com/getsphericon/sphericon/issues/336)
- lift a Suppression instead of deleting it (ADR 0049) ([fb20de7](https://github.com/getsphericon/sphericon/commit/fb20de7f1a4ada0a57756873341b9f35b2a385fe)), closes [#526](https://github.com/getsphericon/sphericon/issues/526)
- load event history through /events/batch with backfill and clientEventId ([f11ed75](https://github.com/getsphericon/sphericon/commit/f11ed75e9c51c853d42bb857eef9efc96db83fbd))
- log in with a Passkey that replaces the password and the Second factor ([e6ac217](https://github.com/getsphericon/sphericon/commit/e6ac217dc067ca4571febb45fdebfcefea4a0da6)), closes [#515](https://github.com/getsphericon/sphericon/issues/515) [#516](https://github.com/getsphericon/sphericon/issues/516) [#517](https://github.com/getsphericon/sphericon/issues/517) [#518](https://github.com/getsphericon/sphericon/issues/518) [#519](https://github.com/getsphericon/sphericon/issues/519) [#520](https://github.com/getsphericon/sphericon/issues/520) [#521](https://github.com/getsphericon/sphericon/issues/521) [#522](https://github.com/getsphericon/sphericon/issues/522)
- look up what a Workspace holds for an email in Privacy requests ([2d095e7](https://github.com/getsphericon/sphericon/commit/2d095e77171d899eead4fc9145e12fea140c714b)), closes [#421](https://github.com/getsphericon/sphericon/issues/421)
- one safe command to upgrade an Instance ([#319](https://github.com/getsphericon/sphericon/issues/319)) ([4077686](https://github.com/getsphericon/sphericon/commit/4077686019e7cf3a8c131ebb384ee141949bb215))
- **operator:** abuse detector with warn and freeze tiers (ADR 0026) ([#251](https://github.com/getsphericon/sphericon/issues/251)) ([999b014](https://github.com/getsphericon/sphericon/commit/999b014186338128dfa39097fcf5da752ce3a93f)), closes [#193](https://github.com/getsphericon/sphericon/issues/193)
- **operator:** billing holds distinguished in banner, Audit log and console ([e109b21](https://github.com/getsphericon/sphericon/commit/e109b211fea84ae33b1ab64413afb07ce4476d3e))
- **operator:** hold set and clear report whether they changed anything ([d0dcd83](https://github.com/getsphericon/sphericon/commit/d0dcd8374a2436cdbf8efea6c53153d1bbfa9d49))
- **operator:** read one Workspace's usage on the service surface ([e6e1d39](https://github.com/getsphericon/sphericon/commit/e6e1d395d7910e0b212a095b4c68d6f0a42de7f2))
- read secrets from files with SPHERICON_NAME_FILE ([61ad50e](https://github.com/getsphericon/sphericon/commit/61ad50ef90187fe05814e3b616fcd2cdca33236c)), closes [#324](https://github.com/getsphericon/sphericon/issues/324)
- refuse deleting a Segment that a draft or scheduled Broadcast uses ([ab37d8b](https://github.com/getsphericon/sphericon/commit/ab37d8b8fc3d3436973797dbcbe34f350a9a1d8f)), closes [#491](https://github.com/getsphericon/sphericon/issues/491)
- refuse deleting a Tag, Integration or Sending domain that live work uses ([2892c73](https://github.com/getsphericon/sphericon/commit/2892c733ece019a78558db003e8c31888805c82f)), closes [#494](https://github.com/getsphericon/sphericon/issues/494)
- refuse history on the live path and reserved actions from customers ([0f4c744](https://github.com/getsphericon/sphericon/commit/0f4c74456ee11fd8270b6ce942ab59cb2fb788aa))
- refuse to start while migrations are pending ([3101ebb](https://github.com/getsphericon/sphericon/commit/3101ebb88471f9d5f4b3395f31797d2921634035))
- rotate the encryption key and the JWT secret ([ba3f20b](https://github.com/getsphericon/sphericon/commit/ba3f20b6f4d32ab63e234cbd7136d63215c7a6b6)), closes [#321](https://github.com/getsphericon/sphericon/issues/321)
- search the contacts list by email ([f94b0e6](https://github.com/getsphericon/sphericon/commit/f94b0e6253ffbc7c80de773d099c75b25adacbd1)), closes [#419](https://github.com/getsphericon/sphericon/issues/419)
- **segments:** archive and unarchive a Segment with the Discard mixin instead of deleting it ([7984e48](https://github.com/getsphericon/sphericon/commit/7984e483bbb3f91d746c9e3878ed16fabc2bc0b3)), closes [#527](https://github.com/getsphericon/sphericon/issues/527)
- self-disabling Webhook endpoints and an HTTP step (ADR 0039) ([#382](https://github.com/getsphericon/sphericon/issues/382)) ([3db2add](https://github.com/getsphericon/sphericon/commit/3db2adda297bfae0e347232b175df805466095b9))
- **sending-domains:** archive and unarchive a Sending domain instead of deleting it ([ab69fec](https://github.com/getsphericon/sphericon/commit/ab69fecfe1948b0babb128b801c8528c2c2cab88)), closes [#529](https://github.com/getsphericon/sphericon/issues/529)
- ship a docker compose file with a one-shot migrate service ([84018f2](https://github.com/getsphericon/sphericon/commit/84018f27390f955274d41e1f38efbea821947955))
- short-lived MCP access tokens with rotating refresh tokens ([4531279](https://github.com/getsphericon/sphericon/commit/4531279f166b5db8c3e92b3125307f9944c9d38f)), closes [#361](https://github.com/getsphericon/sphericon/issues/361)
- show a Setup checklist on the Overview until the first email is sent ([19e6a96](https://github.com/getsphericon/sphericon/commit/19e6a96e2137828e78a347c545350f51472c1daa))
- show a styled page for an address no route matches ([0377629](https://github.com/getsphericon/sphericon/commit/03776290079d59eca5f34a4ca895590b34498078))
- show the DKIM record and a check-now action on a pending Setup checklist step ([88344e6](https://github.com/getsphericon/sphericon/commit/88344e6ddb3ab6460c0d6ca8e6263fd53ff7ce9b)), closes [#401](https://github.com/getsphericon/sphericon/issues/401)
- show the fault of a 500 on the page in development ([fc5b2c6](https://github.com/getsphericon/sphericon/commit/fc5b2c65d5f8e520741bbea5584dcf9f0c77361c))
- sign in with Google and GitHub ([#483](https://github.com/getsphericon/sphericon/issues/483)) ([0331e79](https://github.com/getsphericon/sphericon/commit/0331e79f6cd54b6ed75f85cb2aa74a37dbb1da28)), closes [#434](https://github.com/getsphericon/sphericon/issues/434) [#435](https://github.com/getsphericon/sphericon/issues/435) [#436](https://github.com/getsphericon/sphericon/issues/436) [#437](https://github.com/getsphericon/sphericon/issues/437) [#438](https://github.com/getsphericon/sphericon/issues/438) [#439](https://github.com/getsphericon/sphericon/issues/439) [#440](https://github.com/getsphericon/sphericon/issues/440) [#441](https://github.com/getsphericon/sphericon/issues/441) [#442](https://github.com/getsphericon/sphericon/issues/442)
- sign releases with cosign and attach an SBOM ([a132d7f](https://github.com/getsphericon/sphericon/commit/a132d7fd59a690d63d0bc45b54b3b22951c18458)), closes [#322](https://github.com/getsphericon/sphericon/issues/322)
- sign the User in when the verification link is followed ([#406](https://github.com/getsphericon/sphericon/issues/406)) ([8cc0221](https://github.com/getsphericon/sphericon/commit/8cc02215b33746ebcf6144155f5803521488f4d0))
- **site:** billing link on the hold banner, set by BILLING_URL ([a71e08c](https://github.com/getsphericon/sphericon/commit/a71e08c2e65171e58aa6a698cdffefdada6de4cf))
- split workspace settings into General, Team, Tracking, Sending, Developers and MCP pages ([dd93e75](https://github.com/getsphericon/sphericon/commit/dd93e75452c22368e17bed5f20fd6751f1144791))
- stack notifications and show pending invitations as a tab ([9b25bc9](https://github.com/getsphericon/sphericon/commit/9b25bc98fe3cb9dcc2455d02aecb765e953870d3))
- support Client ID Metadata Documents for MCP clients ([39d65e6](https://github.com/getsphericon/sphericon/commit/39d65e6e6c5d0a242f52c7e6a178f77c57e06074)), closes [#360](https://github.com/getsphericon/sphericon/issues/360)
- take the password at registration and bind the verify link to its browser ([#407](https://github.com/getsphericon/sphericon/issues/407)) ([9c8fc32](https://github.com/getsphericon/sphericon/commit/9c8fc32f629a80656ac0f6587665374aa4d8a09c))
- Tracking domain for the links in a Sending domain's mail (ADR 0038) ([#385](https://github.com/getsphericon/sphericon/issues/385)) ([c9f5177](https://github.com/getsphericon/sphericon/commit/c9f51778b9a5776cdde6f6485ab2d9198ef73dfc))
- **ui:** add a Tags page that deletes a Tag and names what uses it ([bbe8bd1](https://github.com/getsphericon/sphericon/commit/bbe8bd1524ba9f6fa5c05f8ee848fbef7701f558))
- usage metering and operator holds on the Operator surface ([#236](https://github.com/getsphericon/sphericon/issues/236)) ([f512684](https://github.com/getsphericon/sphericon/commit/f512684d0957506046581432a6cc1cc3588697b9)), closes [#224](https://github.com/getsphericon/sphericon/issues/224)

### Bug Fixes

- a Broadcast's claim and its send job are one transaction ([10d28db](https://github.com/getsphericon/sphericon/commit/10d28db0e317e7d11c93f10d900e74bd01f466fe))
- a license key that is not valid yet is refused ([1c97e02](https://github.com/getsphericon/sphericon/commit/1c97e0250a310b302b41ae0936ea19334c34962b))
- a queued plan job cannot claim a Broadcast that was unscheduled after its due check ([98c7f72](https://github.com/getsphericon/sphericon/commit/98c7f7203f9e9c2bb2cd3591b3dff1df2fa22ffa))
- a stale DKIM check no longer reports a second flip of a Sending domain ([4b61e11](https://github.com/getsphericon/sphericon/commit/4b61e11e3e835f9bb251a27b1f59eb5045e46465))
- **accounts:** audit the Verified email an accepted Invitation proves ([bd00e28](https://github.com/getsphericon/sphericon/commit/bd00e282a541cff034609ecd6091d4361b00c936))
- **accounts:** invitation to an unverified address needs name and password (ADR 0030) ([c2c66f3](https://github.com/getsphericon/sphericon/commit/c2c66f3a0a7a5fbb7e93d22d67b096a0f03e026b))
- answer a validator failure with 422, refuse NUL characters and log every 500 ([a778d8f](https://github.com/getsphericon/sphericon/commit/a778d8faed00e544fe4fe220b9c3dd6036c93216))
- apply dev migrations with goose so the start guard sees them ([0c770b9](https://github.com/getsphericon/sphericon/commit/0c770b92dcec2a93734bd9f54034b79fb8e068f7))
- archived Segments, Integrations and Sending domains in rates, new work and tag lookups ([cfc55d6](https://github.com/getsphericon/sphericon/commit/cfc55d62955b8dcefbcec280ff691611e466e95e)), closes [#525](https://github.com/getsphericon/sphericon/issues/525)
- **automations:** lock steps saves against Enroll and keep archived Automations out of status and counts ([980ea5b](https://github.com/getsphericon/sphericon/commit/980ea5b1a284e2fb2fa18c99772315c2027a040b))
- broadcast status is a stateless table so a late plan job cannot regress sent or failed ([13b9216](https://github.com/getsphericon/sphericon/commit/13b921639a4adfddefbe5495867812fb308aa5fb))
- build the nightly race run with cgo ([0ff33a1](https://github.com/getsphericon/sphericon/commit/0ff33a124de0950e665220378e5fa610fd3abb3c))
- check out the tags in the nightly contract job ([f5d6cdf](https://github.com/getsphericon/sphericon/commit/f5d6cdf5036a1e2c8c49a9afd03a4893b50adfd4))
- check the schema per request in development only ([c5f0956](https://github.com/getsphericon/sphericon/commit/c5f0956ab3d4a84f74b18ccb05cc85c21ff87792))
- correct two comments and sign sessions through the verifier's primary service ([fdc310a](https://github.com/getsphericon/sphericon/commit/fdc310a1a3a88e1839c3d37636e454a62581afc4))
- create the database again between the drop and the migrate in db:reset ([613fb7a](https://github.com/getsphericon/sphericon/commit/613fb7afb5754e18f180678225f0717dea9fa755))
- drop the stale pgx pool alert and revert comment, re-wrap a config comment ([1f7e7c7](https://github.com/getsphericon/sphericon/commit/1f7e7c7090ecde2ad313f84997a7dd4a4102abc3))
- drop the used token from the verify-email address so a reload does not offer the form again ([02a8b16](https://github.com/getsphericon/sphericon/commit/02a8b163df2dd4602362a894b7ff61a807fa9e48))
- email verification gates access, closing account pre-hijacking ([#248](https://github.com/getsphericon/sphericon/issues/248)) ([ab47c31](https://github.com/getsphericon/sphericon/commit/ab47c3171ceea5a4813333d9d41077a4e4c9387a))
- **emailrender:** refuse Liquid include and render tags ([#238](https://github.com/getsphericon/sphericon/issues/238)) ([44fa0ae](https://github.com/getsphericon/sphericon/commit/44fa0aedd2baec8c066cf228b3eb7985f18f7b73))
- fence an Automation step's mail and HTTP call against an exit of the Enrollment ([ccd2b08](https://github.com/getsphericon/sphericon/commit/ccd2b088a80bdac9e8b4368f4123442d14ad93dc))
- fill missing 5xx series in the ALB alarm and fix alarm doc references ([eb513f3](https://github.com/getsphericon/sphericon/commit/eb513f3f4b2e4ebbdb2eb129bcad555bde8f1c2e)), closes [#265](https://github.com/getsphericon/sphericon/issues/265)
- give check:infra a Terraform data dir of its own so it needs no AWS credentials ([484d6f4](https://github.com/getsphericon/sphericon/commit/484d6f4cc72b827cae80ace901508200f668584d))
- give empty tables a minimum height so the EmptyState shows ([f50151c](https://github.com/getsphericon/sphericon/commit/f50151caba16a56efe4a3e1b49929b504b268cde))
- harden User deletion after review (adr 0050) ([fb8cefa](https://github.com/getsphericon/sphericon/commit/fb8cefa8ecf80170967ea4ab57f4c5ba62d7e906)), closes [#509](https://github.com/getsphericon/sphericon/issues/509)
- highlight only the current section in the sidebar ([292de40](https://github.com/getsphericon/sphericon/commit/292de40691383e6d119715e34fd487ac13485d63))
- i18n key order, route-glue coverage and a moved path after [#286](https://github.com/getsphericon/sphericon/issues/286) ([#288](https://github.com/getsphericon/sphericon/issues/288)) ([2b32fbb](https://github.com/getsphericon/sphericon/commit/2b32fbbb6ae5112c6852ddc965e6a71470011152))
- **integrations:** stop members redirecting stored SMTP/SES credentials ([#250](https://github.com/getsphericon/sphericon/issues/250)) ([567df4a](https://github.com/getsphericon/sphericon/commit/567df4af4f0e9d1eddcf74532a70d444c47ae5a4))
- keep the Suppression contact index whole so it also serves the workspace foreign key ([b4de2cc](https://github.com/getsphericon/sphericon/commit/b4de2cc90c5f1da53ec3809796af33c92c1447a9))
- keep the upstream status out of metadata fetch errors and document the allowlist effect ([47f907d](https://github.com/getsphericon/sphericon/commit/47f907dd74eb16ae8160f05d68deacb419e28570)), closes [#360](https://github.com/getsphericon/sphericon/issues/360)
- let noNUL pass a request that has no body ([5abe903](https://github.com/getsphericon/sphericon/commit/5abe903f6dc294d9bc0795dc1ce7cb926e86b154))
- list revoked tokens as revoked, honour scope narrowing on refresh, index refresh tokens ([55f12da](https://github.com/getsphericon/sphericon/commit/55f12da7431dda386b934cafa354fda848130bd7))
- log what the WAF counts or blocks so count mode can be reviewed ([29e1206](https://github.com/getsphericon/sphericon/commit/29e12060ff5e5f21ac2f8972d1f3e42bc5a96b16))
- make email verification atomic and the invitation session check authoritative ([18c6663](https://github.com/getsphericon/sphericon/commit/18c6663a7dc1ed572bfa8baaba7c1ad306147162))
- **migrations:** finish the Enrollment version constraints in the definition drop ([aa89d69](https://github.com/getsphericon/sphericon/commit/aa89d6988f6ab396b666fa07a1142812f536bed6)), closes [#489](https://github.com/getsphericon/sphericon/issues/489)
- **migrations:** finish the version constraints in the backfill job and guard the rename ([38b0311](https://github.com/getsphericon/sphericon/commit/38b0311aac3bcceb6f59dbc086dd9202e893133c))
- **migrations:** sort the Automation version migrations after main and index them by the workspace rule ([9cdcfb5](https://github.com/getsphericon/sphericon/commit/9cdcfb51764e46fd2c9fd9f93fafcac84f2f9bb2))
- move the docs link next to the logo in the header ([83c220e](https://github.com/getsphericon/sphericon/commit/83c220e02d5b8f56f59ecc6a140220324161266e))
- name the verification token claims, fold the register paths and keep a retry on verify ([d3ccc15](https://github.com/getsphericon/sphericon/commit/d3ccc15d4eb2f40f33b0324410b1879a61590728))
- one email rule for Privacy requests, validation message and page tests ([36cc749](https://github.com/getsphericon/sphericon/commit/36cc749a0d838f362ef813b472fe643c6166044e)), closes [#420](https://github.com/getsphericon/sphericon/issues/420)
- point check:infra at its own Terraform data dir by an absolute path ([7e92a17](https://github.com/getsphericon/sphericon/commit/7e92a1771bc40b2fe3ee089c4d218c7f06b340b8))
- publish a repeated clientEventId once per batch and tidy the rejection seam ([4019ef5](https://github.com/getsphericon/sphericon/commit/4019ef5ba2bd59707ab24e7c3bbcdd8e0020e7c3))
- refuse a System-sender Test send with no From, cover its caps and checklist step ([5056eed](https://github.com/getsphericon/sphericon/commit/5056eed09b1fd6d4ce76bd1ae3d3ab149ed6b219))
- remove dismissing the Setup checklist ([c3f6424](https://github.com/getsphericon/sphericon/commit/c3f64249be38fe48cf31abb78f7a6423dbaa96db))
- repair the nightly race, contract and vulnerability jobs ([7c38f98](https://github.com/getsphericon/sphericon/commit/7c38f98a503819b1c9f7dda0a3af2eb5d6665415))
- review findings on billing (in-flight sends, billing audit detection, banner) ([#259](https://github.com/getsphericon/sphericon/issues/259)) ([561e439](https://github.com/getsphericon/sphericon/commit/561e4395ccbaf96779a5c030547976984af80d26))
- review findings on the Export streams ([82f5085](https://github.com/getsphericon/sphericon/commit/82f5085a473cd1b26c02221de4551a3e6dc4a342))
- router tests no longer wait on transactions of other databases of the shared server ([7d9588d](https://github.com/getsphericon/sphericon/commit/7d9588d59e74675c07018b9a8a88fab567832a2d))
- saturate the deferral delay instead of overflowing it ([52b46d4](https://github.com/getsphericon/sphericon/commit/52b46d493159b8cad64d9e727757ce898a0f64fc))
- **segments:** bind custom field keys and compile rules to generated ent predicates ([#237](https://github.com/getsphericon/sphericon/issues/237)) ([639dd3b](https://github.com/getsphericon/sphericon/commit/639dd3b63a460ea9dcb7b6c61f6b8e994d31fc55))
- **segments:** the compiler tells rules from groups by kind, not by their fields ([74209ad](https://github.com/getsphericon/sphericon/commit/74209adacc929e91b37847cc844377ebf77ab18b))
- **server:** rely on the global body cap in the hooks handler and pin the /mcp 413 ([#260](https://github.com/getsphericon/sphericon/issues/260)) ([19e8e22](https://github.com/getsphericon/sphericon/commit/19e8e22c2dfb863b8d29b147dfba802e4aa4d1a3))
- sign in from an invitation is a router link, not an onClick navigate ([8531260](https://github.com/getsphericon/sphericon/commit/853126005b48d570d12548120dbea56f1582dafd))
- sign the image with docker_signs all and tighten the verify docs ([bf6afa4](https://github.com/getsphericon/sphericon/commit/bf6afa476e364190733eb56988ec87764d3e3193))
- **site:** declare the 409 of a Workspace restore past its window in the contract ([62f2044](https://github.com/getsphericon/sphericon/commit/62f2044bfae7af029588c338bb8aed7c710ddfa8))
- **staff:** give the erasure requester and the license feature their own names ([#300](https://github.com/getsphericon/sphericon/issues/300)) ([32073bf](https://github.com/getsphericon/sphericon/commit/32073bf7d6c312c0f01500fdbd80c2d60f3786b6))
- state in the contract that an id is up to 18 digits and that every operation can answer 400 and 429 ([aaa6518](https://github.com/getsphericon/sphericon/commit/aaa6518fa618169b9e5857ff92c0385f6829c2a7))
- stop settings titles repeating the page title and keep the quickstart about the product ([b0add13](https://github.com/getsphericon/sphericon/commit/b0add139fa539f91bed1b05feae7881a3ffd5b30))
- tell the Owner why a Tracking domain is stuck, trust loopback only, localize the DKIM notice ([fecede0](https://github.com/getsphericon/sphericon/commit/fecede09824343c00e7879a550bf0ea46d88dd2c))
- usage close isolation, invitation audit and strict segment kind ([6943e64](https://github.com/getsphericon/sphericon/commit/6943e6470c25d58f6813b8640bc4f29ed5b75f68))
- **usage:** bound the service listener, cheapen the contacts peak and share the hold expiry rule ([#249](https://github.com/getsphericon/sphericon/issues/249)) ([43e9389](https://github.com/getsphericon/sphericon/commit/43e93891313bcd35275f376b1534b8b151f9d2cb))
- **usage:** one failing Workspace no longer stops the others from closing ([2724896](https://github.com/getsphericon/sphericon/commit/2724896c2929b444c62be0c8df018405043875d4))
- validate the create-owner email, take email first at every layer, match false case-insensitively ([9329764](https://github.com/getsphericon/sphericon/commit/9329764bd5c2f26fff18131f29e653e64bd91fc8))
- write Sphericon with a capital letter in every user-facing text ([6f81737](https://github.com/getsphericon/sphericon/commit/6f817379e6fd942fd2d64d2a26ec54ffb795c1df))

### Performance Improvements

- **test:** hash token secrets at bcrypt's minimum cost in tests ([#276](https://github.com/getsphericon/sphericon/issues/276)) ([74f1397](https://github.com/getsphericon/sphericon/commit/74f139792458683b3639d4207cd3ca870f8d6235))
- **test:** run test packages in parallel, each on its own database; shuffle only in CI ([#283](https://github.com/getsphericon/sphericon/issues/283)) ([1766f9a](https://github.com/getsphericon/sphericon/commit/1766f9aa3f565a5ec880b334d94aa05d896fea76))

### Code Refactoring

- a broadcast recipient holds no delivery state and figures are not stored ([b5fe04d](https://github.com/getsphericon/sphericon/commit/b5fe04dd1fb2ed24c95e3f4a371ba502dcb1f33b)), closes [#485](https://github.com/getsphericon/sphericon/issues/485)
- archive a sent Broadcast with the Discard mixin instead of deleting it ([ed8ef77](https://github.com/getsphericon/sphericon/commit/ed8ef779737bf5d7a23e80263f4c649317e9194a))
- **automations:** version an Automation's steps ([f052b80](https://github.com/getsphericon/sphericon/commit/f052b8061fd38362f1522b28ecc462b0475dca25))
- index every workspace and foreign key column, drop redundant prefix indexes ([f742920](https://github.com/getsphericon/sphericon/commit/f742920cf8a5263bc0c9c2b469c06b763932f6c3))
- rename the river pool size, drop the queue alias ([3c74824](https://github.com/getsphericon/sphericon/commit/3c74824e3ed7dd346e078d3f2fc8b10bf7c91778))
- ship the English locale only ([c3011b8](https://github.com/getsphericon/sphericon/commit/c3011b85f2d4f2933956e874224cbccb1a4fd531))
- the database enforces every enum, the public id length and the telemetry setting ([4411e87](https://github.com/getsphericon/sphericon/commit/4411e872438e138f208b57bbd9548e2f33606430))
- **workspaces:** name the deletion time discardedAt in the site, staff and service contracts ([a56c7ed](https://github.com/getsphericon/sphericon/commit/a56c7ed7bc4a32404fca39cc8e9492193191c9ad))

## [0.1.2](https://github.com/getsphericon/sphericon/compare/v0.1.1...v0.1.2) (2026-10-11)

### Features

- **docs:** serve the documentation at docs.getsphericon.app ([#220](https://github.com/getsphericon/sphericon/issues/220)) ([884e516](https://github.com/getsphericon/sphericon/commit/884e51696dfaff0e93161ffc2e426046a91373f3))
- **infra:** serve the MCP surface on mcp.getsphericon.app ([f29a81b](https://github.com/getsphericon/sphericon/commit/f29a81b34434e8212e9d7d73c8de37a4f2982fa6))
- **mcp:** serve the MCP resource from an optional dedicated host (MCP_URL) ([72ef0af](https://github.com/getsphericon/sphericon/commit/72ef0afedad7caf09807daf906e5cea028be15a1))
- **operator:** Operator identity, /operator surface and console v1 (ADR 0026) ([#215](https://github.com/getsphericon/sphericon/issues/215)) ([2210e75](https://github.com/getsphericon/sphericon/commit/2210e756d3d67ee58fbd9f4f83f77e2ae9b080dd))

### Bug Fixes

- **infra:** keep RDS backup retention within the AWS free plan limit ([20ba3b1](https://github.com/getsphericon/sphericon/commit/20ba3b1b62b331f25d92215845994a6a84a734a2))

## [0.1.1](https://github.com/getsphericon/sphericon/compare/v0.1.0...v0.1.1) (2026-10-10)

### Features

- **infra:** host on AWS (ECS Fargate, RDS, Route 53, SES), supersede ADR 0028 ([48ad17b](https://github.com/getsphericon/sphericon/commit/48ad17b60977e0fed57459c312bef25c16bd0fe9))
- **infra:** move the product to getsphericon.app, managed entirely by Terraform ([0c24e2c](https://github.com/getsphericon/sphericon/commit/0c24e2c3ace70242beda98b662d13f8d4d6d7ec4))

### Bug Fixes

- **release:** publish the image under the getsphericon organization ([b306731](https://github.com/getsphericon/sphericon/commit/b306731c35581b8427ea601fe4067a7a205234a7))

## 0.1.0 (2026-10-10)

### ⚠ BREAKING CHANGES

- **segments:** remove Segment type; a Segment is always a rule

### Features

- **api:** add-suppression and unsubscribe operations in /api and MCP ([#55](https://github.com/mokevnin/sphericon/issues/55)) ([5bce053](https://github.com/mokevnin/sphericon/commit/5bce05398c3bb84782f49a9cbf5e774785f763f2))
- **api:** broadcast drafts, audience, test-send and report in /api and MCP ([dfd78b2](https://github.com/mokevnin/sphericon/commit/dfd78b23a4595c27bec5837a9a16bcb18c573b09))
- **api:** manage Integrations through the external API ([402306f](https://github.com/mokevnin/sphericon/commit/402306fe14c0d445cda6aa61dc4e131807c0a1f7))
- **api:** manage Sending domains through the external API ([5d0e0b4](https://github.com/mokevnin/sphericon/commit/5d0e0b403a137feca936bf0d3b64d6ec5e1c3243))
- **api:** rails-style resourceful URLs via TypeSpec CRUD templates ([0628507](https://github.com/mokevnin/sphericon/commit/062850790be59c10cdba27c0b20ef724e10fc2d1))
- **api:** read-only custom fields, sending domains and complaint/bounce rates ([#54](https://github.com/mokevnin/sphericon/issues/54)) ([3ebda14](https://github.com/mokevnin/sphericon/commit/3ebda149ff16dc41743337d47c6777878c03a7db))
- **api:** segments CRUD and preview in the external API ([be9cbda](https://github.com/mokevnin/sphericon/commit/be9cbdaa22e933b58755d8f5ef2330041857070f))
- **api:** templates and webhooks in /api ([a50b2a2](https://github.com/mokevnin/sphericon/commit/a50b2a2ab2b4d9219b5624584345f933e9855f5b))
- **apitokens:** one module mints and revokes API tokens; a token cannot grant what it lacks ([2cae730](https://github.com/mokevnin/sphericon/commit/2cae73041f554736c5a624b90308291829a0a8b9))
- **auth:** a single login route; remove the library /auth/ and avatar routes ([30feed3](https://github.com/mokevnin/sphericon/commit/30feed3a6ee20244577f0edcfdfb4810a7e34494))
- **auth:** add login link to register page ([b3f70de](https://github.com/mokevnin/sphericon/commit/b3f70deab821143ca99c6bd526e4551973dc7f6e))
- **auth:** email Users when a Second factor is required and before their grace ends ([6bd3a2a](https://github.com/mokevnin/sphericon/commit/6bd3a2a1cfd658771cab3fcd59df39d2bbcb48c8)), closes [#191](https://github.com/mokevnin/sphericon/issues/191)
- **auth:** enforce session expiry and carry user id and epoch in the session token ([ceb8d75](https://github.com/mokevnin/sphericon/commit/ceb8d754aacaeabef904ed6b1a6524d9abde4808))
- **auth:** enforced session expiry, session epoch, TOTP second factor and two-step login (ADR 0020) ([a54fb7c](https://github.com/mokevnin/sphericon/commit/a54fb7c3cd66360eb67ff94cd06d29b4fc2e4a23))
- **auth:** enforced session expiry, session epoch, TOTP second factor and two-step login (ADR 0020) ([#195](https://github.com/mokevnin/sphericon/issues/195)) ([a54fb7c](https://github.com/mokevnin/sphericon/commit/a54fb7c3cd66360eb67ff94cd06d29b4fc2e4a23))
- **auth:** enroll a TOTP Second factor with Recovery codes ([ad0c3b8](https://github.com/mokevnin/sphericon/commit/ad0c3b843d1869ccbfd84f78a768dca912900a31))
- **auth:** let a withheld Owner or Admin lift the requirement from the SPA ([cb88630](https://github.com/mokevnin/sphericon/commit/cb8863089a617c74b9b71410586c01f003565d1f))
- **auth:** mark the JWT cookie Secure when served over HTTPS ([5d7297d](https://github.com/mokevnin/sphericon/commit/5d7297d9e754bc94041acecebbd76253220e2166))
- **auth:** password reset, email verification, and email change ([b732f56](https://github.com/mokevnin/sphericon/commit/b732f56a58b37994846a8b1218aeab2405384644))
- **auth:** rate limit forgot-password without revealing accounts ([9cbda8e](https://github.com/mokevnin/sphericon/commit/9cbda8efb438d8dda7d9e306a10fc4159359e390))
- **auth:** require a Second factor for a Workspace with a grace period ([32d4e78](https://github.com/mokevnin/sphericon/commit/32d4e788147c282cc4440d2d4c6059925cb5fde6)), closes [#190](https://github.com/mokevnin/sphericon/issues/190)
- **auth:** reset another User's Second factor (Owner/Admin and operator command) ([7a71da0](https://github.com/mokevnin/sphericon/commit/7a71da06ebbb36315f35b47fea5427a8c0d48799)), closes [#192](https://github.com/mokevnin/sphericon/issues/192)
- **auth:** revoke sessions on password change, reset, email change and sign-out-everywhere ([7ebf7ad](https://github.com/mokevnin/sphericon/commit/7ebf7ad4efe1cacc08682418e258d577bd92886a))
- **auth:** self-service account pages + profile email management ([4713e61](https://github.com/mokevnin/sphericon/commit/4713e61dd6208c55518279313fe676a610f0a7da))
- **auth:** throttle login per account with exponential delay ([6291441](https://github.com/mokevnin/sphericon/commit/629144150ce7f5189df03d6da4cf7ecd24630f50))
- **auth:** two-step login with TOTP or Recovery code ([49976b2](https://github.com/mokevnin/sphericon/commit/49976b2c561929210a03095b3840296f565d06c3))
- **automations:** sync workflow builder theme with Mantine color scheme ([1deee46](https://github.com/mokevnin/sphericon/commit/1deee4650008b1659cc7c9602397630d6d612d63))
- **config:** fail fast on weak production JWT secrets and make examples safe ([7fcf520](https://github.com/mokevnin/sphericon/commit/7fcf52026de314056cff5102dbb20b48b7a80a7e))
- **contacts:** emit PII-free contact.erased Event and webhook on Erasure ([eb06f3f](https://github.com/mokevnin/sphericon/commit/eb06f3ffbe84e84f72c7867f862f8a370c245ad9))
- **contacts:** erase a Contact by id (delete is Erasure, ADR 0021) ([8783bbb](https://github.com/mokevnin/sphericon/commit/8783bbb668cb506cb0b05c901433478a548e080f))
- **contacts:** erase by email and by visitor_id ([1d97f28](https://github.com/mokevnin/sphericon/commit/1d97f282f057a0d6bb32bd61812f6a594dd94305))
- **contacts:** stop in-flight work and clear internal queues on erasure ([119f7a4](https://github.com/mokevnin/sphericon/commit/119f7a4fdf45be03369ac7d05e3a76933a24303c))
- **contacts:** subject-access export of one Contact ([32c3d2f](https://github.com/mokevnin/sphericon/commit/32c3d2f770d5a837f48f21a3b190161277204de3))
- **db:** bound Postgres pools, export pool stats, explicit river retention ([816d5c8](https://github.com/mokevnin/sphericon/commit/816d5c8cae8ae7f4f241ca5351c9b77d08b83986))
- **deploy:** add Helm chart with migrate hook and check:helm ([2a3b753](https://github.com/mokevnin/sphericon/commit/2a3b753588fce2c2e924964a2ee9b48eec6c86a7))
- **dev:** one Postgres for all checkouts, started by an idempotent db:up ([d2f295f](https://github.com/mokevnin/sphericon/commit/d2f295f4aa9d7fa81e59236a4f5049a68cf1b623))
- **dev:** per-worktree dev stack ports and origin ([2961828](https://github.com/mokevnin/sphericon/commit/296182885ea4ce53b877c6fe5181456f870ca647))
- **dev:** shared Postgres profile and per-checkout scratch database names ([6d55b8c](https://github.com/mokevnin/sphericon/commit/6d55b8cec30c1b3d18845083f5d64f392dffe209))
- **dev:** worktrees:gc sweeps databases and daemon state of deleted worktrees ([7ee8a95](https://github.com/mokevnin/sphericon/commit/7ee8a9532d549b7294298524a040c5f05f02a093))
- edit Integration send limits and show effective limit and usage ([dc079a3](https://github.com/mokevnin/sphericon/commit/dc079a360327b2d88485c9698f01e4f13ba21c9e))
- **ee:** advanced retention prunes the audit log ([47fe2c0](https://github.com/mokevnin/sphericon/commit/47fe2c0deff07bc552c8b008716b5b1d98e2bd04))
- **ee:** align audit diff keys with the explicit entries (from/to, changed marker) ([d6c351e](https://github.com/mokevnin/sphericon/commit/d6c351ed142c9fe2cc39732c716e7e802dfc0b6e))
- **ee:** audit access and configuration entities ([e0cb2d9](https://github.com/mokevnin/sphericon/commit/e0cb2d9f37157b6daeb418fbaf6a8a1011d16981))
- **ee:** audit capture in the scoped client, Tag first ([a3e448f](https://github.com/mokevnin/sphericon/commit/a3e448f589a88356eb6ab9eebbaafe9beec7b093))
- **ee:** audit content, audience and Contact changes ([47341f3](https://github.com/mokevnin/sphericon/commit/47341f31be0a3af552ca243d0d6d80f65bfc6111))
- **ee:** audit explicit actions (invitation, login, password change, suspension) ([cc474c0](https://github.com/mokevnin/sphericon/commit/cc474c088c6b8fde262365239b1b116c61c9a91e))
- **ee:** audit log (spec [#110](https://github.com/mokevnin/sphericon/issues/110)) ([65b0ae7](https://github.com/mokevnin/sphericon/commit/65b0ae742f8ec01539a3ad3fe0f75e03dcae9cf4))
- **ee:** audit log CSV export and /api read with audit:read scope ([0f80ff6](https://github.com/mokevnin/sphericon/commit/0f80ff6a8393cc1f812fda2140bc1845223086fd))
- **ee:** audit log filters, diff view and change-history links ([dacef11](https://github.com/mokevnin/sphericon/commit/dacef118ee4c1df57f9c3e6c693230533fa39f6a))
- **ee:** audit log retention setting, /api filters, audited export, safer snapshots ([926c63d](https://github.com/mokevnin/sphericon/commit/926c63df2661d41dd63a7428903bbe19a862531d))
- **ee:** audit log skeleton, a role change is recorded and listed ([23db54c](https://github.com/mokevnin/sphericon/commit/23db54c4b83cc7b6e17d3965fd520d633415968b))
- **ee:** audit retention control and audit UI tests ([ab582a1](https://github.com/mokevnin/sphericon/commit/ab582a14a202412ee4e7fafb86b46ef47c0a2356))
- **ee:** forward audit entries to Webhook endpoints ([76cef23](https://github.com/mokevnin/sphericon/commit/76cef23896c45199aac5246567915fae5699a635))
- **events:** delete analytical Events past EVENTS_RETENTION_DAYS ([427de7c](https://github.com/mokevnin/sphericon/commit/427de7c82333cf41eecdd2fb1b82560d6e0287b0))
- **events:** prune the domain-event outbox below the slowest consumer ([68e4d50](https://github.com/mokevnin/sphericon/commit/68e4d50001ecb898116be0168abef74b6174f78e))
- **export:** typed contact export document; river API for test queue access ([66a8fec](https://github.com/mokevnin/sphericon/commit/66a8fec0826cc1105ec6698a0603c17ba3738c80))
- expose Integration effective send limit, source and 24h usage ([8183b7e](https://github.com/mokevnin/sphericon/commit/8183b7e7142820f558940401bf25fcd68d9c3913))
- **external:** automations in /api with tag steps, activate behind a second lock ([6e06db3](https://github.com/mokevnin/sphericon/commit/6e06db32dbe02f4eceb0a9616fa31ddf72c47d0b))
- **external:** batch upsert of contacts and batch record of events ([bd717be](https://github.com/mokevnin/sphericon/commit/bd717be8e45f508f279923f3184980735339b22c))
- **frontend:** migrate Templates to the resource form module ([55906e1](https://github.com/mokevnin/sphericon/commit/55906e1b1aaf1c372f56502bc9046bc95cf6a654))
- **frontend:** resource form lifecycle module, proven on Contacts ([1335b6c](https://github.com/mokevnin/sphericon/commit/1335b6c663c6085e409ec7e7a1f1a9041d3c3380))
- **i18n:** instance-level ru/en/es localization ([4ca2d88](https://github.com/mokevnin/sphericon/commit/4ca2d889777df1d7f2b9c0d63a281d697777a132))
- **infra:** SaaS infra in Terraform ([#214](https://github.com/mokevnin/sphericon/issues/214)) ([27691b2](https://github.com/mokevnin/sphericon/commit/27691b21d2d3c8ee040fa7bdad7ac3b6b31e7d2b))
- **mcp:** add oauth 2.1 for claude.ai connectors ([b9fe50a](https://github.com/mokevnin/sphericon/commit/b9fe50af82496b4781fd2f5de48a347290ad4c4e))
- **mcp:** mark contact-supplied fields as untrusted in tool results ([87bf6e1](https://github.com/mokevnin/sphericon/commit/87bf6e179652f88fe042e815999cc12ea66612e7))
- **mcp:** serve shipped playbooks as MCP prompts and grant authoring scopes to OAuth connectors ([5c9f116](https://github.com/mokevnin/sphericon/commit/5c9f116341efdf3a7d11ee289213f3428879a35b))
- **mcp:** serve the external API as MCP tools at /mcp ([7b14dbb](https://github.com/mokevnin/sphericon/commit/7b14dbb34bab37b9b79a36cdb60f7107b6ccaffc))
- merge ticket-92 metrics listener into spec-91 integration ([045c11f](https://github.com/mokevnin/sphericon/commit/045c11f4f483ca609d4180fb4032001385ee4fde))
- **metrics:** count provider deferrals apart from send errors ([bc085f9](https://github.com/mokevnin/sphericon/commit/bc085f921140189874daa6dc3faec91a37f0e232))
- **metrics:** export outbox lag, queue depth and send outcomes ([b487d7e](https://github.com/mokevnin/sphericon/commit/b487d7eb5416acbb77b51a76b737512be721e75e))
- operations docs, retention, pools, metrics, Helm, security policy ([a6474f0](https://github.com/mokevnin/sphericon/commit/a6474f08a7f2381dadb12ded3b3abc12fcfa840d))
- **outbound:** classify provider too-fast and quota replies as Deferral ([57387c0](https://github.com/mokevnin/sphericon/commit/57387c0c00e36274000fe437db8dff8677ebabe8))
- **outbound:** count transactional sends against the send rate limit ([6b3815a](https://github.com/mokevnin/sphericon/commit/6b3815aebaba5946e2d4e0afcd7655d82d28757b))
- **outbound:** route every send surface through one Outbound send module ([34e150b](https://github.com/mokevnin/sphericon/commit/34e150be510b50aed7c95d28589388c4a4d41ef2))
- pace Broadcast recipients at the Send rate limit and expose progress and ETA ([42f3eec](https://github.com/mokevnin/sphericon/commit/42f3eec86155769b3ed1cd60c4483d445f3eb223)), closes [#123](https://github.com/mokevnin/sphericon/issues/123)
- rate limiting for public and authenticated endpoints (ADR 0018) ([305836b](https://github.com/mokevnin/sphericon/commit/305836b0012a7bcf4dd4004ea30a7d1f1d89b544))
- **ratelimit:** rate limit /api and /mcp per Workspace ([0ff76a1](https://github.com/mokevnin/sphericon/commit/0ff76a1e9eadd3095fe578ef4899a3161df65f77))
- **ratelimit:** rate limit /collect and cap payload size ([0379086](https://github.com/mokevnin/sphericon/commit/0379086e0e3cb26b79ac2b88c7e764b1c4fad10a))
- **ratelimit:** rate limit public human endpoints per IP ([049de64](https://github.com/mokevnin/sphericon/commit/049de64d20192f39353918434583684cf7f561b7))
- **ratelimit:** tracking never refuses a recipient ([3bca8f7](https://github.com/mokevnin/sphericon/commit/3bca8f708d48a7205ef64163ba05b984f4190b1b))
- **segments:** migrate Segment create/edit to the resource form module ([8cf434b](https://github.com/mokevnin/sphericon/commit/8cf434ba8168752b751ae72a4c282a38c92543a3))
- **segments:** remove Segment type; a Segment is always a rule ([9466fa7](https://github.com/mokevnin/sphericon/commit/9466fa7615c98f8a7507f914373b05eca3adf924))
- send lock with broadcasts:send, automations:activate and mcp:send scopes ([2b0f37d](https://github.com/mokevnin/sphericon/commit/2b0f37dc637bd931c3e00442bf9ff78389d6a728))
- send rate limit on Integration with Deferral outcome ([6c9f75b](https://github.com/mokevnin/sphericon/commit/6c9f75baa83b67e8cbf49ae378becd7e56621d7e))
- **sending:** add sending domains with DKIM verification (ADR 0010 slice 1) ([d39b84c](https://github.com/mokevnin/sphericon/commit/d39b84c4b30941bd1ede58592c9c10db0fb4dce6))
- **sending:** double opt-in confirmation, slice 1 (ADR 0013) ([9ce2dead](https://github.com/mokevnin/sphericon/commit/9ce2ded91b10f0b70232e33eb6753177ea705ee6))
- **sending:** gate sending on a verified domain + notify on loss (ADR 0010 slice 3) ([b9f5c93](https://github.com/mokevnin/sphericon/commit/b9f5c933556bc06e5ef553bf3f1e69b0bf79cb7a))
- **sending:** RFC 8058 one-click unsubscribe (ADR 0012) ([0d10d33](https://github.com/mokevnin/sphericon/commit/0d10d332dc22cf33309866f081cc3deedff190b5))
- **sending:** sign outbound mail with native DKIM (ADR 0010 slice 2) ([de4ff7b](https://github.com/mokevnin/sphericon/commit/de4ff7b570479f9db448b67d3ee6f656c049b0d0))
- **sending:** trust seeded sending domains in dev so the send gate doesn't block local mail ([35fad76](https://github.com/mokevnin/sphericon/commit/35fad767a7b4c846b9b6647f597545345006715b))
- **sending:** workspace CAN-SPAM postal address in email footer ([4c575b0](https://github.com/mokevnin/sphericon/commit/4c575b085265d05b2b13981f7033bcf2ab792b34))
- **sendlimit:** discover SES send quota on save and hourly ([5a3c76a](https://github.com/mokevnin/sphericon/commit/5a3c76a846eec11536141a894d94c12d2d60b856))
- serve /metrics on an opt-in internal listener ([868b807](https://github.com/mokevnin/sphericon/commit/868b807073e5b5c22b52b14dc5bd237f7ffad0c4))
- **server:** serve /metrics on an opt-in internal listener ([214d391](https://github.com/mokevnin/sphericon/commit/214d391bf2fbb4d8af0e1a9070fabd25d12fd28f))
- **server:** serve /metrics on an opt-in internal listener ([e6ced4c](https://github.com/mokevnin/sphericon/commit/e6ced4c1711b214680d31ee098888a504ea9d19f))
- **site:** explicit null clears optional fields on Contact and Broadcast update ([a84ad67](https://github.com/mokevnin/sphericon/commit/a84ad67810005d5a5d997c21d8ff3443ca3103ae))
- **spa:** confirm and unsubscribe pages call the generated client ([d92e889](https://github.com/mokevnin/sphericon/commit/d92e88976af5fb93308933320caa2367c2f3497e))
- **spa:** explain 403 on token and OAuth consent actions ([2a746d3](https://github.com/mokevnin/sphericon/commit/2a746d3d5bb1589c9f5bfcdf7a49dfe75433e926))
- **suspension:** show the owner a banner when the workspace is suspended ([7670331](https://github.com/mokevnin/sphericon/commit/7670331e7bd879773cf505452263a1c361d0a369))
- **suspension:** workspace suspend CLI, owner notice, hold UI; fence outbound claims ([c9fb068](https://github.com/mokevnin/sphericon/commit/c9fb068bb85186aa7bc46a9f6c28872970d00921))
- **tags:** tags end to end with has-tag segment rule and MCP tools ([9e1ca94](https://github.com/mokevnin/sphericon/commit/9e1ca94db0c4ac2d921697686a1f9bed2a7a12eb))
- **telemetry:** OpenTelemetry traces + metrics ([78deb33](https://github.com/mokevnin/sphericon/commit/78deb3316967f9cbcba5402a79e416904907e695))
- **ui:** add erase and export actions to the contact view ([94b2235](https://github.com/mokevnin/sphericon/commit/94b22354443a013c20bed89d8ea28034114e449c))
- **ui:** warn when an Integration's provider send quota cannot be read ([71777a5](https://github.com/mokevnin/sphericon/commit/71777a5b757db13c2d137f3bfe9940f961e13057))
- workspace scoping through a generated scoped client (ADR 0017) ([87e9012](https://github.com/mokevnin/sphericon/commit/87e90124fe737e8d7f2e8b2ac361e819d95c4ffc)), closes [#78](https://github.com/mokevnin/sphericon/issues/78)
- **workspaces:** members UI, invite accept flow, and hardening ([86c0b1a](https://github.com/mokevnin/sphericon/commit/86c0b1aa2f63f766e6dedd1c8b11567b933f8049))
- **workspaces:** multi-user memberships and invitations ([fb1439d](https://github.com/mokevnin/sphericon/commit/fb1439db26afc7668be8cc99ce45fc97133eece4))

### Bug Fixes

- **api:** do not expose which members have a Second factor ([88d6ff9](https://github.com/mokevnin/sphericon/commit/88d6ff9450d5f8c7bb7d7288f1c10abff9220249))
- audit path list follows the suspension package ([09001c8](https://github.com/mokevnin/sphericon/commit/09001c8dddb3cafca71151190295b42088061d5e))
- **auth:** atomic per-address reset budget and uniform forgot-password work ([5b751b9](https://github.com/mokevnin/sphericon/commit/5b751b93d793fdb22dea1924a005547532ca5c7d))
- **auth:** harden the Second factor flows after the [#143](https://github.com/mokevnin/sphericon/issues/143) review ([678d06c](https://github.com/mokevnin/sphericon/commit/678d06ce4fda5865fec2816718dd903246ca7b05))
- **auth:** satisfy errcheck on the authtoken HMAC write ([b9d6330](https://github.com/mokevnin/sphericon/commit/b9d63301aaca9115f277ffc2e2cde35a0df8e2bb))
- **automations:** deleting an automation deletes its enrollments ([14e2766](https://github.com/mokevnin/sphericon/commit/14e2766db697ea3d032cfdf3f2e031d9ea69b77a))
- **broadcasts:** bind delayed send job to its schedule ([#77](https://github.com/mokevnin/sphericon/issues/77)) ([840c8f4](https://github.com/mokevnin/sphericon/commit/840c8f42fb40bd3bf9608f96c57cb86586af2461))
- **consent:** a failed consent write is a 5xx, a deleted workspace is a no-op ([87faaba](https://github.com/mokevnin/sphericon/commit/87faabad45e1594398663f68290508e71f228f39))
- **consent:** tell an expired confirmation link from an invalid one ([7a0e748](https://github.com/mokevnin/sphericon/commit/7a0e748fbd1bdb11386e9bf3419dbc18746104bc))
- **contacts:** adapt in-flight erasure to the operator-aware API after merge ([ace1f06](https://github.com/mokevnin/sphericon/commit/ace1f06803abaeda0336f2f8b753ec03da30798e))
- **contacts:** dedicated erase role check, shared export lookup and server-named download ([39f68ec](https://github.com/mokevnin/sphericon/commit/39f68ec586da9ba40f69cec8382362e4c1d5ef69))
- **contacts:** erasure clears queued webhook deliveries and keeps recipients_total consistent ([329ddb2](https://github.com/mokevnin/sphericon/commit/329ddb2ea41688824c64fd54b3f92684fb7e229f))
- count only awake recipients in a Deferral's backlog ([4bebbb1](https://github.com/mokevnin/sphericon/commit/4bebbb19b57bce6aaa981028a4e642b53b0e6645))
- **deps:** make realclientip-go a direct dependency (go mod tidy) ([a0e3d5d](https://github.com/mokevnin/sphericon/commit/a0e3d5d1a53e4d35bd8e9bf53cc414487c00ceb5))
- **dev:** per-instance Caddy admin and HTTP ports, stricter worktrees:gc ([aecad32](https://github.com/mokevnin/sphericon/commit/aecad32c81f400778d3d02b575070bf4bb131043))
- **dev:** use the host's global pnpm store instead of one inside the repo ([2c3f9c7](https://github.com/mokevnin/sphericon/commit/2c3f9c71fc5ee31c605b4a34eff24c038ae66a8a))
- **docs:** render the VitePress theme templates instead of raw placeholders ([2852fbe](https://github.com/mokevnin/sphericon/commit/2852fbeba5d8ef51cc1326feadc8847d6ab18881))
- **docs:** render the VitePress theme templates instead of raw placeholders ([048e82a](https://github.com/mokevnin/sphericon/commit/048e82a74867cb8be5220df21e2f338e3d735641))
- **e2e:** isolate metrics port, reuse app Accounts, share mail wait helper ([0ab63f9](https://github.com/mokevnin/sphericon/commit/0ab63f95320d36fb2783ae361c65abc3ae786a37))
- **e2e:** keep a cut-short poll from masking the inbox listing ([6bba9c2](https://github.com/mokevnin/sphericon/commit/6bba9c2d9371e62dfd8415eee449ae24b33c1fb1))
- **e2e:** keep RequireNone's lookup error off t while Never's condition runs ([4291e53](https://github.com/mokevnin/sphericon/commit/4291e534d146de096696eb53b4f34386fcdd242d))
- **ee:** mask the Operator identity in forwarded audit entries ([c7d16b7](https://github.com/mokevnin/sphericon/commit/c7d16b7cd84eae4566708c5b1647b142aee58a87))
- eventlog list tests follow the optional Event fields from main ([8a9832d](https://github.com/mokevnin/sphericon/commit/8a9832da020aca3af62edb530c3a2b9396c999f0))
- **external:** an explicit null on contact update clears the field ([fca1b72](https://github.com/mokevnin/sphericon/commit/fca1b7231f8c30ab207effc74470153cf94b50ba))
- **external:** complete the token scope enum and workspace-scope /api token management ([a872bc5](https://github.com/mokevnin/sphericon/commit/a872bc572699d75abba8f3f761cff462813bd710))
- **external:** publish ContactCreated for contacts created through /api ([9c24027](https://github.com/mokevnin/sphericon/commit/9c2402765730502f22df3fd431ca421f9cdb7219))
- **fixtures:** add missing invitations fixture ([7f5c01f](https://github.com/mokevnin/sphericon/commit/7f5c01f724cbeadde95a62b166a8822197dfe6d9))
- **fixtures:** drop the removed segments.type column from the Globex fixture ([86129b2](https://github.com/mokevnin/sphericon/commit/86129b257e9fcf37cf2aa5207db6b18aacb012fa))
- **fixtures:** register inviteHash template func for invitation fixtures ([5cde016](https://github.com/mokevnin/sphericon/commit/5cde016ca82bd66fbb701180b18705dfd5af71fb))
- **frontend:** address review findings for resource form module ([408be80](https://github.com/mokevnin/sphericon/commit/408be8017bfb62c703215e667efea5576496727b))
- harden the e2e Inbox and the app lifecycle after review ([b6b2a97](https://github.com/mokevnin/sphericon/commit/b6b2a97af80e53dbe0e894e3ef1f4571ca68015c))
- **logs:** drop email and phone from log statements ([1bd72ec](https://github.com/mokevnin/sphericon/commit/1bd72ecda899f6d1cd69c087b4f536166f504484))
- look through the metrics decorator for the quota reader; count both new tables ([97ccf16](https://github.com/mokevnin/sphericon/commit/97ccf16c3da4a9a517148edc1ec1fdfc39d8253a))
- **oauth:** consent and token management need owner or admin; validate RFC 8707 resource ([2d87867](https://github.com/mokevnin/sphericon/commit/2d8786715593b2e189a606be8017e72a7303a9e3))
- **ops:** address review findings on metrics, retention and migrations ([83b1bc4](https://github.com/mokevnin/sphericon/commit/83b1bc4dd20f584fee4964e9b6427b1c929da5e1))
- pace and estimate by the effective Send rate limit, provider quota included ([5e80a4d](https://github.com/mokevnin/sphericon/commit/5e80a4d76a4d92aad7bbd40da23b9ea4b2f41284))
- **ratelimit:** cap collect events before decoding, wait-aware recovery 429 ([f82bcca](https://github.com/mokevnin/sphericon/commit/f82bccaf47908ff8d196561582489a7da631adf1))
- refuse to start when the job queue cannot start ([4bcce5f](https://github.com/mokevnin/sphericon/commit/4bcce5f53016ba850f7310516284573354425eb1))
- scoped update Modify cannot assign workspace_id ([8914dfb](https://github.com/mokevnin/sphericon/commit/8914dfb1e4c171b933f0bb73058902325be86869))
- **security:** explicit SameSite=Lax and cross-origin protection on cookie APIs ([4e960b0](https://github.com/mokevnin/sphericon/commit/4e960b0b7ba9283bdd9c883b2fe230c08520ecc5))
- **segments:** make Segment definition non-nullable in the contract ([88d0dc0](https://github.com/mokevnin/sphericon/commit/88d0dc0c033af2b3aa16ecbf10d0b0943c0e6811))
- **server:** cap request body size and add read timeouts ([8ec54a9](https://github.com/mokevnin/sphericon/commit/8ec54a9e3cbca11ca5f447b6ee1e517800908a71))
- **site:** name and cover contact alias-key and custom-field clearing ([51d06ef](https://github.com/mokevnin/sphericon/commit/51d06ef229f99f8de93a49f52c44c3e1193fe733))
- **telemetry:** address review findings on the metrics listener ([9992f1b](https://github.com/mokevnin/sphericon/commit/9992f1b49e61478fd6b67cf648e1904e20dfd1df))
- transactional sends never defer; rename throttle vocabulary to busy; honest quota discovery response ([1e321c1](https://github.com/mokevnin/sphericon/commit/1e321c11a8e347122f1848e287f342a9a14638d1))
- **ui:** give each error toast a case-specific fallback message ([cad7cdd](https://github.com/mokevnin/sphericon/commit/cad7cdd221ff3d923531c71bad363d9830267187))
- **ui:** stop error toasts repeating the title as the message ([76f9593](https://github.com/mokevnin/sphericon/commit/76f959333abd588d5f80d0e3b2dd716b4313c6bf))
