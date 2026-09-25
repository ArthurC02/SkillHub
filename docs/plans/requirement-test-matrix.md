# 需求 ID × 測試對照表

`03` 的 `RELEASE-001` 要的就是這張表：MVP 必要的每一個需求 ID，對到證明它的具名測試，或說出為什麼沒有。

主語是 `02` 裡**沒有標「後 MVP」**的需求 ID。標了後 MVP 的（`TEST-003`、`TEST-004`、`SEC-004`）不在這張表上，寫上去會被擋。

`automation-check` 的 `requirement-test-matrix` 逐列對帳：ID 不得漏列或多列、狀態必須是下面五個詞之一、每個反引號裡的名字必須真的出現在某個測試檔裡（或本身就是一個存在的檔案路徑）、`有測試` 的列不得有缺口、其餘四種狀態的列必須寫出缺口。

| 狀態 | 意思 |
| --- | --- |
| `有測試` | 該 ID 的允收準則都有具名測試在證明 |
| `部分` | 一部分準則有測試，另一部分沒有——缺口那一欄說出是哪一部分 |
| `待真機` | 剩下那半只能在生產同規格節點上量，不是測試能證的 |
| `待真人` | 剩下那半是人工審閱或人的回應時間 |
| `未實作` | 功能本身還不存在，所以測試也不會有 |

**這張表不是允收本身**：一列寫 `有測試` 只代表有東西在問那個問題，不代表那個問題被問對了。測試本身的品質由 `istqb-test-design` 與突變證明守。

## 目錄與搜尋

| 需求 ID | 狀態 | 具名測試 | 缺口 |
| --- | --- | --- | --- |
| DISC-001 | 有測試 | `TestBlankQueryReturnsNoResults`、`TestOffTopicQueryIsRefusedWithASuggestion`、`DISC-001: search hits the public endpoint, which needs no session` | |
| DISC-002 | 有測試 | `DISC-002: a result row carries all seven columns, and infers none of them`、`TestFilterDimensionsWithoutDataAreRejectedNotIgnored` | |
| DISC-003 | 有測試 | `TestAnonymousReadsCatalogSkillDetail` | |
| DISC-004 | 有測試 | `DISC-009 comparison gives absent fields their actual state`、`DISC-009 相容性不同的兩個 Skill,那一列要說有差異` | |
| DISC-005 | 未實作 | | 結構化意圖抽取與查詢改寫在 `discovery` 套件完全沒有程式碼，`Embed` 直接吃原句 |
| DISC-006 | 有測試 | `TestBrowseCatalogScopeOrderFiltersShapeAndNoModelCall`、`TestCategoryFiltersTheCatalogAndNamesTheAbsence`、`TestTheCatalogResponseDropsEveryFieldABrowseCouldOnlyFillWithAConstant`、`DISC-006: an empty catalog is distinct from a failed catalog read` | |

## 匯入與驗證

| 需求 ID | 狀態 | 具名測試 | 缺口 |
| --- | --- | --- | --- |
| SKILL-001 | 有測試 | `TestARefusedImportAnswersInTheLanguageOfTheScreen`、`TestDuplicateDetectionCannotReachAnotherWorkspacesVersion` | |
| SKILL-002 | 有測試 | `TestCategorizeSeparatesBySeverity`、`TestFrontmatterRules` | |
| SKILL-003 | 有測試 | `TestEmbeddedCodeIsDisclosed`、`TestEmbeddedCodeBoundaryLines` | |
| SKILL-004 | 有測試 | `TestLicenseProvenancePrecedence` | |
| SKILL-005 | 有測試 | `TestURLDisclosuresAggregateByHost` | |
| SKILL-006 | 未實作 | | Agent Plugin 匯入在 `apps/platform` 與 `apps/web` 都沒有程式碼；裁定已下，實作未落地 |

## 工作區與試跑

| 需求 ID | 狀態 | 具名測試 | 缺口 |
| --- | --- | --- | --- |
| WS-001 | 有測試 | `TestForkCatalogSkillIntoCallerWorkspace`、`WS-001 the detail page lists the versions, newest first, with the oldest saying why it has no comparison` | |
| WS-002 | 有測試 | `TestTestCaseCRUDAndPromptValidation`、`TestALoggedInStrangerGetsNothingFromAnotherWorkspacesResources` | |
| TEST-001 | 有測試 | `TestTestCaseCRUDAndPromptValidation`、`02:TEST-001 第 3 條 a user can add, delete and withdraw the confirmation of a criterion` | |
| TEST-002 | 有測試 | `02:TEST-002 the upload rules are on screen before anything is uploaded`、`02:TEST-002 without the rules there is no upload control at all` | |
| TEST-005 | 有測試 | `02:TEST-005 the summary discloses every required item before the run starts`、`02:TEST-005 a permission change forces a fresh confirmation instead of reusing the old one` | |
| RUN-001 | 有測試 | `TestIncompatibleWorkIsRefusedBeforeItIsQueued`、`TestRetryAddsAttemptWithoutOverwritingTheProviderMapping` | |
| RUN-002 | 有測試 | `TestRunWalksTheStateMachineAndIsCleanedUp`、`TestARefusedTeardownIsRecordedAsFailedAndCleaningUpAgainIsSafe` | |
| RUN-003 | 有測試 | `TestThePermissionSummarySaysWhatTheTokenCeilingDependsOn`、`TestTheTokenCeilingAbortMessageSaysTheRoundsDependOnToolCalls` | |
| RUN-004 | 有測試 | `TestCancelReachesTheProviderAndStopsTheRun`、`TestSupervisorTimesOutARunThatOutlivedItsWallClock`、`TestRetriesAreBoundedAndClassifiedAsProviderFailure` | |
| TRACE-001 | 有測試 | `TestTraceIngestionMasksBeforeStorageAndDedupesOnResend`、`TestAdvancedViewNamesMissingEventsAndRefusesToLookComplete` | |

## 評估與改善

| 需求 ID | 狀態 | 具名測試 | 缺口 |
| --- | --- | --- | --- |
| EVAL-001 | 有測試 | `TestEvaluationIsRecordedWithVerifiedEvidenceAndNeverTouchesTheRun`、`TestFeedbackIsRecordedAndCanBeChanged` | |
| EVAL-002 | 有測試 | `TestAcceptedSuggestionsBecomeOneNewVersionAndLeaveTheOldOneAlone`、`TestTwoSuggestionsOnTheSameFileApplyOneAndSayWhyTheOtherDidNot` | |
| EVAL-003 | 有測試 | `TestComparisonShowsBothVerdictsCostsAndTheVersionDiffLink`、`TestRerunningTheSameTestCaseOnANewVersionGoesThroughPreflight` | |
| EVAL-013 | 部分 | `test_a_passed_verdict_on_an_incomplete_trace_is_downgraded`、`test_a_verdict_citing_an_unresolvable_reference_is_downgraded` | 降級機制有測試；一份用這把尺量出來的完整回歸讀數（符合率、逐筆歸因）還沒跑過 |

## 打包與下載

| 需求 ID | 狀態 | 具名測試 | 缺口 |
| --- | --- | --- | --- |
| PACK-001 | 有測試 | `TestTheLicenceAuthorAndProvenanceFilesTravelInEveryTargetsPackage`、`TestTheDownloadGateAnswersEveryRedistributionValue`、`TestBuildAndVersionControlResidueNeverTravels` | |
| PACK-002 | 有測試 | `TestInstallInstructionsStateTheSupportStatusAndAtLeastOneCheck`、`TestInstallInstructionsListWhatTheScriptsImportWithoutDeclaring`、`TestTheProfilesDeclareEnvVarsWithoutCredentials` | |

## 內容與策展

| 需求 ID | 狀態 | 具名測試 | 缺口 |
| --- | --- | --- | --- |
| CONTENT-001 | 部分 | `TestCurationTierNeedsBothHalvesOfTheRecord`、`TestTheTierFilterSeparatesTheReviewedFromTheRest` | 九項精選檢查、白名單准入與數量目標是策展文件的允收，不是程式行為 |
| CONTENT-002 | 有測試 | `TestLicenseProvenancePrecedence`、`TestOperatorRedistributionVerdictIsGovernedLikeTheHold` | |
| CONTENT-003 | 部分 | `TestDependencyExtraction` | 候選清單本身（repo URL、pin SHA、九項檢查值、來源多樣性）是一份策展文件 |
| CONTENT-004 | 部分 | `TestEachGateRefusesAPackageAndSaysWhich`、`TestOperatorRedistributionVerdictIsGovernedLikeTheHold` | monorepo 逐目錄判定授權、逐 repo 查核日期與方法的紀錄沒有測試 |
| CONTENT-005 | 待真人 | | 白話摘要的人工審核工序；`tools/content/review_summaries.py` 沒有任何測試檔，證據是一份一次性報告 |
| CONTENT-006 | 部分 | `TestCategorizeSeparatesBySeverity`、`TestSecretsBlockWithoutEchoingValue`、`TestEmbeddedCodeIsDisclosed` | 精選檢查④「無 eval／動態下載／外連 subprocess」的機械量測沒有程式化掃描 |
| CONTENT-007 | 有測試 | `TestOnlyCuratedTestCasesTravelAndTheRestAreNamed` | |
| CONTENT-008 | 部分 | `TestCurationTierNeedsBothHalvesOfTheRecord` | 「精選標記需要至少一次符合的基準 Run」這條規則本身沒有測試；證據是一份實測報告 |
| CONTENT-009 | 有測試 | `TestSourceContentChangeIsAuditedOnceAndOnlyOnAChange`、`TestSourceAvailabilityIsAuditedOnlyWhenItChanges` | |

## 生成與互動創作

| 需求 ID | 狀態 | 具名測試 | 缺口 |
| --- | --- | --- | --- |
| GEN-001 | 有測試 | `TestGeneratedSkillLandsAsAVersionWithItsOwnProvenance`、`TestAGenerationTheBalanceCannotStartIsRefusedBeforeTheGateway`、`TestABlankTaskDescriptionIsRefusedWithAdvice` | |
| GEN-002 | 有測試 | `TestGeneratedFrontmatterHasNoLicence`、`TestAGeneratedSkillIsNotFoundBySearchIncludingItsOwnCreator`、`TestAForkOfAGeneratedSkillStaysGenerated` | |
| GEN-003 | 有測試 | `TestAGeneratedAnswerPassesTheImportValidator`、`TestGenerationRetriesExactlyOnce`、`TestPossibleSecretIsNotRetried` | |
| GEN-004 | 有測試 | `TestTheGenerationEntryPointIsInvisibleUntilItIsExposed`、`test_an_empty_body_is_refused_not_packaged` | |
| GEN-005 | 有測試 | `TestADiagramOnlyGenerationIsCreated`、`TestExactlyTheDiagramSizeCapIsNotRefused`、`TestADisallowedDiagramMediaTypeIs400` | |
| GEN-006 | 有測試 | `TestFourReferencesIsRefusedBeforeTheGateway`、`TestAReferenceToAnotherUsersPrivateSkillIs422`、`TestALongReferenceIsCutToLeaveRoomForTheMarker` | |
| GEN-007 | 有測試 | `TestChangedConfirmedBriefCannotProduceDraft`、`TestDiagramInterpretationRequiresConfirmedDescriptionAndEveryAnswer` | |
| GEN-008 | 部分 | `TestActConfirmDiagram`、`TestActConfirmReferences`、`TestUnavailableReferenceBlocksDraft` | 流程圖組的 uncertainties 在 Go 沒有閘門，`confirmed` 的純函式邏輯成立不代表上層呼叫方不能繞過 |
| GEN-009 | 有測試 | `TestTheDraftIsCopiedAsThePreviousDraftBeforeTheCommandRuns`、`TestActConfirmFetch`、`TestActDeclineFetch` | |
| GEN-010 | 有測試 | `TestSavingNeedsAConfirmedUnblockedDraftWithTheSameHash`、`TestAnExistingCandidateIsSavedWithoutMaterializingAgain` | |
| GEN-011 | 有測試 | `TestCreationBatchForeignSessionIDIsNotAnOracle` | |
| GEN-012 | 有測試 | `TestCreationBudgetOutOfBandNamesTheBand`、`TestCreationLimitsEndpoint`、`TestCreationMeasureFifteenSessionsAgainstSingleShot` | |

## 可攜與乾淨測試模式

| 需求 ID | 狀態 | 具名測試 | 缺口 |
| --- | --- | --- | --- |
| PORT-001 | 有測試 | `tools/pglite/verify.mjs` | |
| PORT-003 | 有測試 | `PORT-003: with clean_mode on, the notice states its five absences`、`PORT-003: without the flag, the notice renders nothing` | |
| PORT-004 | 有測試 | `TestRequireDBGuardCatchesAPackageThatIgnoresTheSwitch`、`TestTheRealRepositoryPassesIsTheOnePeopleWillSee` | |
| PORT-005 | 有測試 | `TestApplyCleanModePoolLeavesProductionAlone`、`TestCleanModeHandlerLeavesProductionAlone` | |
| PORT-007 | 有測試 | `TestCollectSeedEntriesReadsBothRealBatches`、`TestSeedCleanExcludesTheUnvalidatablePackageAndSaysWhy` | |
| PORT-008 | 有測試 | `TestTheRealRepositoryHasNoSecondDataLayer`、`TestSecondDataLayerProblemsReadsCodeNotComments` | |
| PORT-009 | 有測試 | `TestInProcessDoesNotAuthorize`、`TestPresignedGrantIsShortLivedUnforgeableAndSingleDirection` | |
| PORT-010 | 有測試 | `TestReapsWholeProcessTree`、`TestTheCleanTestModeOnlyRunsCuratedMaterial`、`TestTheReleaseListIsNeverEvenReadOutsideTheCleanTestMode` | |

## Credit 與計費

| 需求 ID | 狀態 | 具名測試 | 缺口 |
| --- | --- | --- | --- |
| CRED-001 | 有測試 | `TestAnOperatorCorrectionLowersTheBalance`、`TestChargeIsIdempotentOnSessionRevision`、`db/tests/immutability_test.sql` | |
| CRED-002 | 有測試 | `TestBilledMicrosRoundsUpNeverDown`、`TestUnknownUsageIsRecordedAndNeverDebited`、`db/tests/immutability_test.sql` | |
| CRED-003 | 有測試 | `TestCanAffordStepEnforcesDebtFloor`、`TestARunIsRefusedWhenTheBalanceCannotCoverItsCeiling`、`TestARunIsChargedWhatItSpentAndOnlyOnce` | |
| CRED-004 | 有測試 | `TestCanStartUsesP95WithMarkupWhenEnoughSamples`、`TestCanStartFallsBackBelowMinSamples` | |
| CRED-005 | 有測試 | `TestCostEventNeverCarriesTheQueryText`、`TestAnonymousSearchStillRecordsCostWithBothIdsNull`、`TestSearchEmbeddingRecordsExactlyOneCostEvent` | |
| CRED-006 | 有測試 | `TestAnEndedCreationSessionLeavesExactlyOneCostSummary`、`TestEveryScheduledJobHasAWorker`、`TestTheDailyStatisticsSurviveAWindowWithNoEvents` | |
| CRED-007 | 有測試 | `TestGrantCreditsIsInvisibleWithoutTheOperatorRole`、`TestGrantRefusesAZeroAmountOrABlankReasonAndWritesNoEntry`、`TestASuccessfulGrantIsAuditedWithItsTargetWorkspace` | |
| CRED-008 | 有測試 | `TestAccountDeletionLeavesTheCreditLedgerAlone` | |

## 後台與維運

| 需求 ID | 狀態 | 具名測試 | 缺口 |
| --- | --- | --- | --- |
| OPS-001 | 有測試 | `TestBackOfficeReadsAnswerAMemberLikeAMissingPage`、`TestMeTellsTheCallerWhetherTheyAreAnOperator` | |
| OPS-002 | 有測試 | `TestAccountLookupAuditsAHitAndNothingElse` | |
| OPS-003 | 有測試 | `TestCreditLedgerShowsTheGrantAndAuditsEveryRead` | |
| OPS-004 | 有測試 | `TestGovernanceLookupReachesPrivateAndTakenDownSkillsButNotDeleted` | |
| OPS-005 | 有測試 | `TestRostersShowWhatIsInForce`、`TestP1HaltStopsBothEntryPointsAndPreservesTheScene` | |
| OPS-006 | 有測試 | `TestOperatorAuditLogListsOnlyOperatorActions` | |
| OPS-007 | 有測試 | `TestCostStatisticsShowTheNewestWindowOfEachKind` | |
| OPS-008 | 有測試 | `TestTrendRangeIsTheLastNUTCDaysEndingToday`、`TestCostTrendSumsEachKindPerUTCDayFromTheFirstInstantOfTheRange`、`TestOperatorActionTrendCountsOnlyOperatorActions` | |
| OPS-009 | 有測試 | `TestTheOperatorsCeilingIsWhatTheNextCallIsGiven`、`TestEveryChangeLeavesOneAuditEventNamingTheReasonAndBothValues`、`TestARefusedChangeLeavesNeitherASettingNorAnEvent` | |

## 非功能需求

| 需求 ID | 狀態 | 具名測試 | 缺口 |
| --- | --- | --- | --- |
| NFR-001 | 部分 | `TestTheNamedEndpointsAreRateLimitedWhenALimiterIsConfigured`、`TestTheRefusalCarriesRetryAfterAndASentence`、`NFR-001: a clean scan says what the scan found and carries the rider that it is not safety`、`NFR-001: a clean row in a list carries the same rider as the detail page` | 目錄列與詳情頁的但書有測試；打包頁乾淨掃描時永遠看得見的判定行沒有但書，那句話被折進 `<details>`（`05` R-90），所以那一面還沒有可以斷言的對象 |
| NFR-002 | 有測試 | `TestTraceIngestionMasksBeforeStorageAndDedupesOnResend`、`TestARunArtifactCanBeListedAndDeletedOnItsOwn` | |
| NFR-003 | 有測試 | `TestAProviderThatCannotBeReachedIsUnavailableRatherThanRefusing`、`TestAnUnrecognisedAndOldSandboxIsStillAnOrphan`、`TestSupervisorRecoversARunThatHasNoJob` | |
| NFR-004 | 待真機 | | 搜尋 p95、建立 Run、Trace 上畫面的秒數門檻，規格自己寫「需在確認基礎設施後校準」 |
| NFR-005 | 部分 | `TestEveryMeasurementTheObservabilityRequirementNamesReachesAScrape`、`TestStatusClassSplitsEveryRangeTheProviderRecorderActsOn`、`TestObserveSinceMeasuresTheElapsedTimeNotTheWallClock`、`TestTheBacklogObserverPublishesEachBacklogsAgeAndReportsTheOnesItCouldNotRead` | 第 2 條的七項量測都有測試證明它們以對的名稱與標籤抓得到；還沒有測試斷言它們在對的呼叫點被寫入（`skill/discovery/service.go:152`、`trial/execution/statemachine.go:250`／`259`／`261`、`trial/execution/provider.go:302`） |
| NFR-006 | 有測試 | `TestProviderContract`、`TestProviderRefusalIsClassifiedAndNotRetried` | |
| NFR-007 | 有測試 | `NFR-007: 搜尋 → 詳情 → 打包，全程鍵盤可達`、`NFR-007: 沒選檔案就按上傳，說的是下一步而不是錯誤碼`、`NFR-007: 空白的回報被擋下來時說得出要補什麼` | |

## 安全

| 需求 ID | 狀態 | 具名測試 | 缺口 |
| --- | --- | --- | --- |
| SEC-001 | 待真人 | | 威脅模型交付物的內容完整性是人工審閱，repo 內沒有對帳這份文件結構的機制 |
| SEC-002 | 部分 | `TestAPackageThatCouldNotBeScannedIsNotTreatedAsACleanOne`、`TestABlockedPackageIsRefusedAndNamesWhatBlockedIt`、`TestAPoolThatMayRecoverIsToldApartFromOneThatCouldNeverRunTheRequest` | 46 項基線全數 pass 要在生產同規格節點上驗；程式層只證得了已落地的判斷邏輯 |
| SEC-003 | 有測試 | `TestFetchRefusesHostResolvingToLoopback`、`TestFetchRefusesMetadataAddressBothSpellings`、`TestFetchRedirectLimit` | |
| SEC-005 | 有測試 | `TestRevokeIsIdempotent`、`TestPresignedGrantIsShortLivedUnforgeableAndSingleDirection` | |
| SEC-006 | 有測試 | `TestARunArtifactCanBeListedAndDeletedOnItsOwn`、`TestPresignedURLStatesItsExpiryAndBindsItsMethod` | |
| SEC-007 | 部分 | `TestTakedownRemovesSkillFromPublicSurface` | 上游「失效」的判準尚未定值（`05` R-88），這半部沒有可證的對象 |
| SEC-008 | 部分 | `TestALoggedInStrangerGetsNothingFromAnotherWorkspacesResources`、`TestANodeReportingAP02BreachHaltsTheFleetWithoutAnOperator` | 測試證的是平台收到探針訊號後的反應；探針在真節點上真的擋得住連線，要在那台節點上驗 |
| SEC-009 | 待真機 | `tools/sec009/t1-escape-attempts.sh`、`tools/sec009/t2-syscall-fuzz.sh`、`tools/sec009/gvisor-smoke.sh` | 46 項全 pass、0 unknown 的判定要在生產同規格節點跑滿前置條件後才成立；CI 跑的是程序檢查 |
| SEC-010 | 部分 | `TestMaskingStoppedHaltsDispatchWithoutAnOperator`、`TestReconcilerStallHaltsDispatchWithoutAnOperator`、`TestANodeReportingAP02BreachHaltsTheFleetWithoutAnOperator` | 「1 小時內接手」與通知真的送達是人的回應與外部系統，測試證不了 |
| SEC-011 | 有測試 | `TestOperatorRosterIsAudited`、`TestAnonymousCallersGetThePublicSurfaceAndNothingElse`、`TestOperatorRedistributionVerdictIsGovernedLikeTheHold` | |
| SEC-013 | 有測試 | `TestInjectionCorpusEvaluationCasesAreCaughtAtTheGoLayer`、`test_tool_observation_is_fenced_and_its_closing_tag_stripped`、`no assistant message can produce a link or an image, whatever it writes` | |

## 學習與觀測

| 需求 ID | 狀態 | 具名測試 | 缺口 |
| --- | --- | --- | --- |
| O11Y-004 | 有測試 | `TestCollectionIsOffWithoutARetentionPeriod`、`TestQueryScriptBuckets`、`TestAFreshlyMintedSessionIdIsOfferedNotUsed` | |
