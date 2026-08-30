package i18n

// Message keys for i18n translations
// Use these constants instead of hardcoded strings

// Common error messages
const (
	MsgInvalidParams     = "common.invalid_params"
	MsgDatabaseError     = "common.database_error"
	MsgRetryLater        = "common.retry_later"
	MsgGenerateFailed    = "common.generate_failed"
	MsgNotFound          = "common.not_found"
	MsgUnauthorized      = "common.unauthorized"
	MsgForbidden         = "common.forbidden"
	MsgInvalidId         = "common.invalid_id"
	MsgIdEmpty           = "common.id_empty"
	MsgFeatureDisabled   = "common.feature_disabled"
	MsgOperationSuccess  = "common.operation_success"
	MsgOperationFailed   = "common.operation_failed"
	MsgUpdateSuccess     = "common.update_success"
	MsgUpdateFailed      = "common.update_failed"
	MsgCreateSuccess     = "common.create_success"
	MsgCreateFailed      = "common.create_failed"
	MsgDeleteSuccess     = "common.delete_success"
	MsgDeleteFailed      = "common.delete_failed"
	MsgAlreadyExists     = "common.already_exists"
	MsgNameCannotBeEmpty = "common.name_cannot_be_empty"
	MsgBatchTooMany      = "common.batch_too_many"
)

// Auth middleware messages
const (
	MsgAuthNotLoggedIn           = "auth.not_logged_in"
	MsgAuthAccessTokenInvalid    = "auth.access_token_invalid"
	MsgAuthUserInfoInvalid       = "auth.user_info_invalid"
	MsgAuthUserIdNotProvided     = "auth.user_id_not_provided"
	MsgAuthUserIdFormatError     = "auth.user_id_format_error"
	MsgAuthUserIdMismatch        = "auth.user_id_mismatch"
	MsgAuthUserBanned            = "auth.user_banned"
	MsgAuthInsufficientPrivilege = "auth.insufficient_privilege"
)

// Token related messages
const (
	MsgTokenNameTooLong          = "token.name_too_long"
	MsgTokenQuotaNegative        = "token.quota_negative"
	MsgTokenQuotaExceedMax       = "token.quota_exceed_max"
	MsgTokenGenerateFailed       = "token.generate_failed"
	MsgTokenGetInfoFailed        = "token.get_info_failed"
	MsgTokenExpiredCannotEnable  = "token.expired_cannot_enable"
	MsgTokenExhaustedCannotEable = "token.exhausted_cannot_enable"
	MsgTokenInvalid              = "token.invalid"
	MsgTokenNotProvided          = "token.not_provided"
	MsgTokenExpired              = "token.expired"
	MsgTokenExhausted            = "token.exhausted"
	MsgTokenStatusUnavailable    = "token.status_unavailable"
	MsgTokenDbError              = "token.db_error"
)

// Redemption related messages
const (
	MsgRedemptionNameLength        = "redemption.name_length"
	MsgRedemptionCountPositive     = "redemption.count_positive"
	MsgRedemptionCountMax          = "redemption.count_max"
	MsgRedemptionCreateFailed      = "redemption.create_failed"
	MsgRedemptionInvalid           = "redemption.invalid"
	MsgRedemptionUsed              = "redemption.used"
	MsgRedemptionExpired           = "redemption.expired"
	MsgRedemptionFailed            = "redemption.failed"
	MsgRedemptionNotProvided       = "redemption.not_provided"
	MsgRedemptionExpireTimeInvalid = "redemption.expire_time_invalid"
)

// User related messages
const (
	MsgUserPasswordLoginDisabled     = "user.password_login_disabled"
	MsgUserRegisterDisabled          = "user.register_disabled"
	MsgUserPasswordRegisterDisabled  = "user.password_register_disabled"
	MsgUserUsernameOrPasswordEmpty   = "user.username_or_password_empty"
	MsgUserUsernameOrPasswordError   = "user.username_or_password_error"
	MsgUserEmailOrPasswordEmpty      = "user.email_or_password_empty"
	MsgUserExists                    = "user.exists"
	MsgUserNotExists                 = "user.not_exists"
	MsgUserDisabled                  = "user.disabled"
	MsgUserSessionSaveFailed         = "user.session_save_failed"
	MsgUserRequire2FA                = "user.require_2fa"
	MsgUserEmailVerificationRequired = "user.email_verification_required"
	MsgUserVerificationCodeError     = "user.verification_code_error"
	MsgUserInputInvalid              = "user.input_invalid"
	MsgUserNoPermissionSameLevel     = "user.no_permission_same_level"
	MsgUserNoPermissionHigherLevel   = "user.no_permission_higher_level"
	MsgUserCannotCreateHigherLevel   = "user.cannot_create_higher_level"
	MsgUserCannotDeleteRootUser      = "user.cannot_delete_root_user"
	MsgUserCannotDisableRootUser     = "user.cannot_disable_root_user"
	MsgUserCannotDemoteRootUser      = "user.cannot_demote_root_user"
	MsgUserAlreadyAdmin              = "user.already_admin"
	MsgUserAlreadyCommon             = "user.already_common"
	MsgUserAdminCannotPromote        = "user.admin_cannot_promote"
	MsgUserOriginalPasswordError     = "user.original_password_error"
	MsgUserInviteQuotaInsufficient   = "user.invite_quota_insufficient"
	MsgUserTransferQuotaMinimum      = "user.transfer_quota_minimum"
	MsgUserTransferSuccess           = "user.transfer_success"
	MsgUserTransferFailed            = "user.transfer_failed"
	MsgUserTopUpProcessing           = "user.topup_processing"
	MsgUserRegisterFailed            = "user.register_failed"
	MsgUserDefaultTokenFailed        = "user.default_token_failed"
	MsgUserAffCodeEmpty              = "user.aff_code_empty"
	MsgUserEmailEmpty                = "user.email_empty"
	MsgUserGitHubIdEmpty             = "user.github_id_empty"
	MsgUserDiscordIdEmpty            = "user.discord_id_empty"
	MsgUserOidcIdEmpty               = "user.oidc_id_empty"
	MsgUserWeChatIdEmpty             = "user.wechat_id_empty"
	MsgUserTelegramIdEmpty           = "user.telegram_id_empty"
	MsgUserTelegramNotBound          = "user.telegram_not_bound"
	MsgUserLinuxDOIdEmpty            = "user.linux_do_id_empty"
	MsgUserQuotaChangeZero           = "user.quota_change_zero"
)

// Quota related messages
const (
	MsgQuotaNegative        = "quota.negative"
	MsgQuotaExceedMax       = "quota.exceed_max"
	MsgQuotaInsufficient    = "quota.insufficient"
	MsgQuotaWarningInvalid  = "quota.warning_invalid"
	MsgQuotaThresholdGtZero = "quota.threshold_gt_zero"
)

// Subscription related messages
const (
	MsgSubscriptionNotEnabled       = "subscription.not_enabled"
	MsgSubscriptionTitleEmpty       = "subscription.title_empty"
	MsgSubscriptionPriceNegative    = "subscription.price_negative"
	MsgSubscriptionPriceMax         = "subscription.price_max"
	MsgSubscriptionPurchaseLimitNeg = "subscription.purchase_limit_negative"
	MsgSubscriptionQuotaNegative    = "subscription.quota_negative"
	MsgSubscriptionGroupNotExists   = "subscription.group_not_exists"
	MsgSubscriptionResetCycleGtZero = "subscription.reset_cycle_gt_zero"
	MsgSubscriptionPurchaseMax      = "subscription.purchase_max"
	MsgSubscriptionInvalidId        = "subscription.invalid_id"
	MsgSubscriptionInvalidUserId    = "subscription.invalid_user_id"
)

// Payment related messages
const (
	MsgPaymentNotConfigured    = "payment.not_configured"
	MsgPaymentMethodNotExists  = "payment.method_not_exists"
	MsgPaymentCallbackError    = "payment.callback_error"
	MsgPaymentCreateFailed     = "payment.create_failed"
	MsgPaymentStartFailed      = "payment.start_failed"
	MsgPaymentAmountTooLow     = "payment.amount_too_low"
	MsgPaymentStripeNotConfig  = "payment.stripe_not_configured"
	MsgPaymentWebhookNotConfig = "payment.webhook_not_configured"
	MsgPaymentPriceIdNotConfig = "payment.price_id_not_configured"
	MsgPaymentCreemNotConfig   = "payment.creem_not_configured"
)

// Topup related messages
const (
	MsgTopupNotProvided    = "topup.not_provided"
	MsgTopupOrderNotExists = "topup.order_not_exists"
	MsgTopupOrderStatus    = "topup.order_status"
	MsgTopupFailed         = "topup.failed"
	MsgTopupInvalidQuota   = "topup.invalid_quota"
)

// Channel related messages
const (
	MsgChannelNotExists          = "channel.not_exists"
	MsgChannelIdFormatError      = "channel.id_format_error"
	MsgChannelNoAvailableKey     = "channel.no_available_key"
	MsgChannelGetListFailed      = "channel.get_list_failed"
	MsgChannelGetTagsFailed      = "channel.get_tags_failed"
	MsgChannelGetKeyFailed       = "channel.get_key_failed"
	MsgChannelGetOllamaFailed    = "channel.get_ollama_failed"
	MsgChannelQueryFailed        = "channel.query_failed"
	MsgChannelNoValidUpstream    = "channel.no_valid_upstream"
	MsgChannelUpstreamSaturated  = "channel.upstream_saturated"
	MsgChannelGetAvailableFailed = "channel.get_available_failed"
)

// Model related messages
const (
	MsgModelNameEmpty     = "model.name_empty"
	MsgModelNameExists    = "model.name_exists"
	MsgModelIdMissing     = "model.id_missing"
	MsgModelGetListFailed = "model.get_list_failed"
	MsgModelGetFailed     = "model.get_failed"
	MsgModelResetSuccess  = "model.reset_success"
)

// Vendor related messages
const (
	MsgVendorNameEmpty  = "vendor.name_empty"
	MsgVendorNameExists = "vendor.name_exists"
	MsgVendorIdMissing  = "vendor.id_missing"
)

// Group related messages
const (
	MsgGroupNameTypeEmpty = "group.name_type_empty"
	MsgGroupNameExists    = "group.name_exists"
	MsgGroupIdMissing     = "group.id_missing"
)

// Checkin related messages
const (
	MsgCheckinDisabled     = "checkin.disabled"
	MsgCheckinAlreadyToday = "checkin.already_today"
	MsgCheckinFailed       = "checkin.failed"
	MsgCheckinQuotaFailed  = "checkin.quota_failed"
)

// Passkey related messages
const (
	MsgPasskeyCreateFailed  = "passkey.create_failed"
	MsgPasskeyLoginAbnormal = "passkey.login_abnormal"
	MsgPasskeyUpdateFailed  = "passkey.update_failed"
	MsgPasskeyInvalidUserId = "passkey.invalid_user_id"
	MsgPasskeyVerifyFailed  = "passkey.verify_failed"
)

// 2FA related messages
const (
	MsgTwoFANotEnabled    = "twofa.not_enabled"
	MsgTwoFAUserIdEmpty   = "twofa.user_id_empty"
	MsgTwoFAAlreadyExists = "twofa.already_exists"
	MsgTwoFARecordIdEmpty = "twofa.record_id_empty"
	MsgTwoFACodeInvalid   = "twofa.code_invalid"
)

// Rate limit related messages
const (
	MsgRateLimitReached      = "rate_limit.reached"
	MsgRateLimitTotalReached = "rate_limit.total_reached"
)

// Setting related messages
const (
	MsgSettingInvalidType      = "setting.invalid_type"
	MsgSettingWebhookEmpty     = "setting.webhook_empty"
	MsgSettingWebhookInvalid   = "setting.webhook_invalid"
	MsgSettingEmailInvalid     = "setting.email_invalid"
	MsgSettingBarkUrlEmpty     = "setting.bark_url_empty"
	MsgSettingBarkUrlInvalid   = "setting.bark_url_invalid"
	MsgSettingGotifyUrlEmpty   = "setting.gotify_url_empty"
	MsgSettingGotifyTokenEmpty = "setting.gotify_token_empty"
	MsgSettingGotifyUrlInvalid = "setting.gotify_url_invalid"
	MsgSettingUrlMustHttp      = "setting.url_must_http"
	MsgSettingSaved            = "setting.saved"
)

// Deployment related messages (io.net)
const (
	MsgDeploymentNotEnabled     = "deployment.not_enabled"
	MsgDeploymentIdRequired     = "deployment.id_required"
	MsgDeploymentContainerIdReq = "deployment.container_id_required"
	MsgDeploymentNameEmpty      = "deployment.name_empty"
	MsgDeploymentNameTaken      = "deployment.name_taken"
	MsgDeploymentHardwareIdReq  = "deployment.hardware_id_required"
	MsgDeploymentHardwareInvId  = "deployment.hardware_invalid_id"
	MsgDeploymentApiKeyRequired = "deployment.api_key_required"
	MsgDeploymentInvalidPayload = "deployment.invalid_payload"
	MsgDeploymentNotFound       = "deployment.not_found"
)

// Performance related messages
const (
	MsgPerfDiskCacheCleared = "performance.disk_cache_cleared"
	MsgPerfStatsReset       = "performance.stats_reset"
	MsgPerfGcExecuted       = "performance.gc_executed"
)

// Ability related messages
const (
	MsgAbilityDbCorrupted   = "ability.db_corrupted"
	MsgAbilityRepairRunning = "ability.repair_running"
)

// OAuth related messages
const (
	MsgOAuthInvalidCode     = "oauth.invalid_code"
	MsgOAuthGetUserErr      = "oauth.get_user_error"
	MsgOAuthAccountUsed     = "oauth.account_used"
	MsgOAuthUnknownProvider = "oauth.unknown_provider"
	MsgOAuthStateInvalid    = "oauth.state_invalid"
	MsgOAuthNotEnabled      = "oauth.not_enabled"
	MsgOAuthUserDeleted     = "oauth.user_deleted"
	MsgOAuthUserBanned      = "oauth.user_banned"
	MsgOAuthBindSuccess     = "oauth.bind_success"
	MsgOAuthAlreadyBound    = "oauth.already_bound"
	MsgOAuthConnectFailed   = "oauth.connect_failed"
	MsgOAuthTokenFailed     = "oauth.token_failed"
	MsgOAuthUserInfoEmpty   = "oauth.user_info_empty"
	MsgOAuthTrustLevelLow   = "oauth.trust_level_low"
)

// Model layer error messages (for translation in controller)
const (
	MsgRedeemFailed          = "redeem.failed"
	MsgCreateDefaultTokenErr = "user.create_default_token_error"
	MsgUuidDuplicate         = "common.uuid_duplicate"
	MsgInvalidInput          = "common.invalid_input"
)

// Distributor related messages
const (
	MsgDistributorInvalidRequest          = "distributor.invalid_request"
	MsgDistributorInvalidChannelId        = "distributor.invalid_channel_id"
	MsgDistributorChannelDisabled         = "distributor.channel_disabled"
	MsgDistributorAffinityChannelDisabled = "distributor.affinity_channel_disabled"
	MsgDistributorTokenNoModelAccess      = "distributor.token_no_model_access"
	MsgDistributorTokenModelForbidden     = "distributor.token_model_forbidden"
	MsgDistributorModelNameRequired       = "distributor.model_name_required"
	MsgDistributorInvalidPlayground       = "distributor.invalid_playground_request"
	MsgDistributorGroupAccessDenied       = "distributor.group_access_denied"
	MsgDistributorGetChannelFailed        = "distributor.get_channel_failed"
	MsgDistributorNoAvailableChannel      = "distributor.no_available_channel"
	MsgDistributorInvalidMidjourney       = "distributor.invalid_midjourney_request"
	MsgDistributorInvalidParseModel       = "distributor.invalid_request_parse_model"
)

// Custom OAuth provider related messages
const (
	MsgCustomOAuthNotFound          = "custom_oauth.not_found"
	MsgCustomOAuthSlugEmpty         = "custom_oauth.slug_empty"
	MsgCustomOAuthSlugExists        = "custom_oauth.slug_exists"
	MsgCustomOAuthNameEmpty         = "custom_oauth.name_empty"
	MsgCustomOAuthHasBindings       = "custom_oauth.has_bindings"
	MsgCustomOAuthBindingNotFound   = "custom_oauth.binding_not_found"
	MsgCustomOAuthProviderIdInvalid = "custom_oauth.provider_id_field_invalid"
)

// Custom channel usage and quota messages
const (
	MsgChannelUsageChannelQuotaReset      = "channel_usage.channel_quota_reset"
	MsgChannelUsageKeyQuotaReset          = "channel_usage.key_quota_reset"
	MsgChannelUsageKeyQuotaUpdated        = "channel_usage.key_quota_updated"
	MsgChannelUsageInvalidRequest         = "channel_usage.invalid_request"
	MsgChannelUsageKeyQuotaNegative       = "channel_usage.key_quota_negative"
	MsgChannelUsageChannelIdsRequired     = "channel_usage.channel_ids_required"
	MsgChannelUsageMaxBatch               = "channel_usage.max_batch"
	MsgChannelUsageChannelIdInvalid       = "channel_usage.channel_id_invalid"
	MsgChannelUsageNotMultiKey            = "channel_usage.not_multi_key"
	MsgChannelUsageKeyFingerprintInvalid  = "channel_usage.key_fingerprint_invalid"
	MsgChannelUsageRecordNotFound         = "channel_usage.record_not_found"
	MsgChannelQuotaLimitNegative          = "channel_quota.limit_negative"
	MsgChannelQuotaModeInvalid            = "channel_quota.mode_invalid"
	MsgChannelQuotaSingleKeyUnsupported   = "channel_quota.single_key_unsupported"
	MsgChannelQuotaResetRequired          = "channel_quota.reset_required"
	MsgChannelKeyQuotaResetRequired       = "channel_quota.key_reset_required"
	MsgChannelCodexKeyInvalidJSON         = "channel.codex_key_invalid_json"
	MsgChannelCodexKeyAccessTokenRequired = "channel.codex_key_access_token_required"
	MsgChannelCodexKeyAccountIDRequired   = "channel.codex_key_account_id_required"
	MsgChannelCodexRefreshFailed          = "channel.codex_refresh_failed"
	MsgChannelCodexRefreshSuccess         = "channel.codex_refresh_success"
	MsgChannelKeysChannelNotFound         = "channel_keys.channel_not_found"
	MsgChannelKeysNotMultiKey             = "channel_keys.not_multi_key"
	MsgChannelKeysDisableIndexRequired    = "channel_keys.disable_index_required"
	MsgChannelKeysEnableIndexRequired     = "channel_keys.enable_index_required"
	MsgChannelKeysDeleteIndexRequired     = "channel_keys.delete_index_required"
	MsgChannelKeysIndexOutOfRange         = "channel_keys.index_out_of_range"
	MsgChannelKeysDisabled                = "channel_keys.disabled"
	MsgChannelKeysEnabled                 = "channel_keys.enabled"
	MsgChannelKeysEnabledCount            = "channel_keys.enabled_count"
	MsgChannelKeysNoEnabledKeys           = "channel_keys.no_enabled_keys"
	MsgChannelKeysDisabledCount           = "channel_keys.disabled_count"
	MsgChannelKeysLastKeyDeleteForbidden  = "channel_keys.last_key_delete_forbidden"
	MsgChannelKeysDeleted                 = "channel_keys.deleted"
	MsgChannelKeysNoAutoDisabledKeys      = "channel_keys.no_auto_disabled_keys"
	MsgChannelKeysDeletedAutoCount        = "channel_keys.deleted_auto_count"
	MsgChannelKeysResetAllCount           = "channel_keys.reset_all_count"
	MsgChannelKeysUnsupportedAction       = "channel_keys.unsupported_action"
)

// Monitor group and public status messages
const (
	MsgMonitorGroupCreated                 = "monitor_group.created"
	MsgMonitorGroupUpdated                 = "monitor_group.updated"
	MsgMonitorGroupDeleted                 = "monitor_group.deleted"
	MsgMonitorGroupRunning                 = "monitor_group.running"
	MsgMonitorGroupRunStarted              = "monitor_group.run_started"
	MsgMonitorGroupRunnerNotStarted        = "monitor_group.runner_not_started"
	MsgMonitorGroupIdInvalid               = "monitor_group.id_invalid"
	MsgMonitorGroupNameInvalid             = "monitor_group.name_invalid"
	MsgMonitorGroupKeyInvalid              = "monitor_group.key_invalid"
	MsgMonitorGroupKeyImmutable            = "monitor_group.key_immutable"
	MsgMonitorGroupModelDescriptionInvalid = "monitor_group.model_description_invalid"
	MsgMonitorGroupIntervalInvalid         = "monitor_group.interval_invalid"
	MsgMonitorGroupTimeoutInvalid          = "monitor_group.timeout_invalid"
	MsgMonitorGroupDegradedInvalid         = "monitor_group.degraded_invalid"
	MsgMonitorGroupChannelRequired         = "monitor_group.channel_required"
	MsgMonitorGroupChannelsMissing         = "monitor_group.channels_missing"
	MsgMonitorGroupNoProbeModels           = "monitor_group.no_probe_models"
	MsgMonitorGroupNoCommonModels          = "monitor_group.no_common_models"
	MsgMonitorGroupModelsNotSupported      = "monitor_group.models_not_supported"
	MsgMonitorGroupInvalidStatus           = "monitor_group.invalid_status"
	MsgMonitorGroupInvalidLimit            = "monitor_group.invalid_limit"
	MsgMonitorGroupInvalidDays             = "monitor_group.invalid_days"
	MsgMonitorStatusDaysInvalid            = "monitor_status.days_invalid"
	MsgMonitorStatusNotFound               = "monitor_status.not_found"
)

// Operations and model performance messages
const (
	MsgOpsInvalidMetric         = "ops.invalid_metric"
	MsgOpsInvalidStartTimestamp = "ops.invalid_start_timestamp"
	MsgOpsInvalidEndTimestamp   = "ops.invalid_end_timestamp"
	MsgOpsEndBeforeStart        = "ops.end_before_start"
	MsgOpsRangeTooLarge         = "ops.range_too_large"
	MsgOpsInvalidChannelType    = "ops.invalid_channel_type"
	MsgOpsInvalidChannelId      = "ops.invalid_channel_id"
	MsgOpsUnassignedChannel     = "ops.unassigned_channel"
	MsgPerfMetricsHoursInvalid  = "perf_metrics.hours_invalid"
	MsgPerfMetricsModelRequired = "perf_metrics.model_required"
	MsgPerfMetricsSummaryFailed = "perf_metrics.summary_failed"
	MsgPerfMetricsQueryFailed   = "perf_metrics.query_failed"
)

// System event query and localized event messages
const (
	MsgSystemEventInvalidStartTimestamp = "system_event.invalid_start_timestamp"
	MsgSystemEventInvalidEndTimestamp   = "system_event.invalid_end_timestamp"
	MsgSystemEventEndBeforeStart        = "system_event.end_before_start"
	MsgSystemEventMonitorProbeStarted   = "system_event.monitor_probe_started"
	MsgSystemEventMonitorChannelFailed  = "system_event.monitor_probe_channel_load_failed"
	MsgSystemEventMonitorSaveFailed     = "system_event.monitor_probe_save_failed"
	MsgSystemEventMonitorFinishFailed   = "system_event.monitor_probe_finish_update_failed"
	MsgSystemEventMonitorNoTargets      = "system_event.monitor_probe_no_targets"
	MsgSystemEventMonitorCompleted      = "system_event.monitor_probe_completed"
	MsgSystemEventChannelAutoDisabled   = "system_event.channel_auto_disabled"
	MsgSystemEventChannelAutoRestored   = "system_event.channel_auto_restored"
	MsgSystemEventKeyQuotaExhausted     = "system_event.channel_key_quota_exhausted"
	MsgSystemEventChannelQuotaExhausted = "system_event.channel_quota_exhausted"
	MsgSystemEventOpsFlushRecovered     = "system_event.ops_flush_recovered"
	MsgSystemEventOpsFlushFailed        = "system_event.ops_flush_failed"
	MsgSystemEventPerfFlushRecovered    = "system_event.perf_flush_recovered"
	MsgSystemEventPerfFlushFailed       = "system_event.perf_flush_failed"
	MsgSystemEventUpstreamFinalFailure  = "system_event.upstream_final_failure"
)
