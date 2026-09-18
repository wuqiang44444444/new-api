package service

import "strings"

// Localized summary CSV header labels (plan 9.2 / 12.5). Keys are stable
// column names; values are frozen per export job language. Labels mirrored
// from the frontend locale files keep the server file aligned with the
// retired browser download; machine-only columns carry authored labels.

var customerExportSummaryLabels = map[string]map[string]string{
	"currency":           {"en": "Currency", "zh": "币种"},
	"estimate_reasons":   {"en": "Estimate reasons", "zh": "估算未完成原因"},
	"group_name":         {"en": "Billing group", "zh": "计费组", "zh-TW": "計費群組", "fr": "Groupe de facturation", "ru": "Группа тарификации", "ja": "課金グループ", "vi": "Nhóm tính phí"},
	"group_ratio_source": {"en": "Group ratio source", "zh": "组倍率来源", "zh-TW": "群組倍率來源", "fr": "Origine du coefficient de groupe", "ru": "Источник коэффициента группы", "ja": "グループ倍率の適用元", "vi": "Nguồn hệ số nhóm"},
	"field_version": {
		"en":    "Field version",
		"zh":    "字段版本",
		"zh-TW": "欄位版本",
		"fr":    "Version du champ",
		"ru":    "Версия поля",
		"ja":    "フィールドバージョン",
		"vi":    "Phiên bản trường",
	},
	"job_id": {
		"en":    "Job ID",
		"zh":    "任务 ID",
		"zh-TW": "任務 ID",
		"fr":    "ID de tâche",
		"ru":    "ID задачи",
		"ja":    "ジョブ ID",
		"vi":    "ID công việc",
	},
	"export_type": {
		"en":    "Export type",
		"zh":    "导出类型",
		"zh-TW": "匯出類型",
		"fr":    "Type d'exportation",
		"ru":    "Тип экспорта",
		"ja":    "エクスポート種別",
		"vi":    "Loại xuất",
	},
	"generated_at": {
		"en":    "Generated at",
		"zh":    "生成时间",
		"zh-TW": "產生時間",
		"fr":    "Généré le",
		"ru":    "Время формирования",
		"ja":    "生成日時",
		"vi":    "Thời gian tạo",
	},
	"period_start": {
		"en":    "Period start",
		"zh":    "周期开始",
		"zh-TW": "週期開始",
		"fr":    "Début de période",
		"ru":    "Начало периода",
		"ja":    "期間の開始",
		"vi":    "Bắt đầu kỳ",
	},
	"period_end": {
		"en":    "Period end",
		"zh":    "周期结束",
		"zh-TW": "週期結束",
		"fr":    "Fin de période",
		"ru":    "Конец периода",
		"ja":    "期間の終了",
		"vi":    "Kết thúc kỳ",
	},
	"timezone": {
		"en":    "Timezone",
		"zh":    "时区",
		"zh-TW": "時區",
		"fr":    "Fuseau horaire",
		"ru":    "Часовой пояс",
		"ja":    "タイムゾーン",
		"vi":    "Múi giờ",
	},
	"customer_id": {
		"en":    "Customer ID",
		"zh":    "客户 ID",
		"zh-TW": "客戶 ID",
		"fr":    "ID du client",
		"ru":    "ID клиента",
		"ja":    "顧客 ID",
		"vi":    "ID khách hàng",
	},
	"customer_username": {
		"en":    "Customer",
		"zh":    "客户",
		"zh-TW": "客戶",
		"fr":    "Client",
		"ru":    "Клиент",
		"ja":    "顧客",
		"vi":    "Khách hàng",
	},
	"row_type": {
		"en":    "Row type",
		"zh":    "行类型",
		"zh-TW": "列類型",
		"fr":    "Type de ligne",
		"ru":    "Тип строки",
		"ja":    "行の種類",
		"vi":    "Loại dòng",
	},
	"api_key_id": {
		"en":    "API Key ID",
		"zh":    "API 密钥 ID",
		"zh-TW": "API 金鑰 ID",
		"fr":    "ID de clé API",
		"ru":    "ID API-ключа",
		"ja":    "API キー ID",
		"vi":    "ID khóa API",
	},
	"api_key_name": {
		"en":    "API Key",
		"zh":    "API 密钥",
		"zh-TW": "API 金鑰",
		"fr":    "Clé API",
		"ru":    "Ключ API",
		"ja":    "APIキー",
		"vi":    "Khóa API",
	},
	"model": {
		"en":    "Model",
		"zh":    "模型",
		"zh-TW": "模型",
		"fr":    "Modèle",
		"ru":    "Модель",
		"ja":    "モデル",
		"vi":    "Mô hình",
	},
	"billing_mode": {
		"en":    "Billing mode",
		"zh":    "计费方式",
		"zh-TW": "計費方式",
		"fr":    "Mode de facturation",
		"ru":    "Способ тарификации",
		"ja":    "課金方式",
		"vi":    "Phương thức tính phí",
	},
	"requests": {
		"en":    "Requests",
		"zh":    "请求数",
		"zh-TW": "請求數",
		"fr":    "Requêtes",
		"ru":    "Запросы",
		"ja":    "リクエスト",
		"vi":    "Yêu cầu",
	},
	"input_tokens": {
		"en":    "Input tokens",
		"zh":    "输入 token",
		"zh-TW": "輸入 token",
		"fr":    "Jetons d’entrée",
		"ru":    "Входные токены",
		"ja":    "入力トークン",
		"vi":    "Token đầu vào",
	},
	"cache_read_tokens": {
		"en":    "Cache read tokens",
		"zh":    "缓存读取 Token",
		"zh-TW": "快取讀取 Token",
		"fr":    "Tokens lus du cache",
		"ru":    "Токены чтения кэша",
		"ja":    "キャッシュ読取 Token",
		"vi":    "Token đọc cache",
	},
	"cache_write_tokens": {
		"en":    "Cache write tokens",
		"zh":    "缓存写入 Token",
		"zh-TW": "快取寫入 Token",
		"fr":    "Tokens écrits en cache",
		"ru":    "Токены записи кэша",
		"ja":    "キャッシュ書込 Token",
		"vi":    "Token ghi cache",
	},
	"output_tokens": {
		"en":    "Output tokens",
		"zh":    "输出 token",
		"zh-TW": "輸出 token",
		"fr":    "Jetons de sortie",
		"ru":    "Выходные токены",
		"ja":    "出力トークン",
		"vi":    "Token đầu ra",
	},
	"billable_calls": {
		"en":    "Billable calls",
		"zh":    "计费次数",
		"zh-TW": "計費次數",
		"fr":    "Appels facturables",
		"ru":    "Оплачиваемые вызовы",
		"ja":    "課金回数",
		"vi":    "Lượt tính phí",
	},
	"refunded_calls": {
		"en":    "Refunded calls",
		"zh":    "退款次数",
		"zh-TW": "退款次數",
		"fr":    "Appels remboursés",
		"ru":    "Возвращённые вызовы",
		"ja":    "返金回数",
		"vi":    "Lượt hoàn phí",
	},
	"original_quota": {
		"en":    "Estimated list price",
		"zh":    "估算原价",
		"zh-TW": "估算原價",
		"fr":    "Prix initial estimé",
		"ru":    "Расчётная сумма до скидок",
		"ja":    "推定割引前金額",
		"vi":    "Giá gốc ước tính",
	},
	"discount_quota": {
		"en":    "Estimated savings",
		"zh":    "估算优惠",
		"zh-TW": "估算優惠",
		"fr":    "Remise estimée",
		"ru":    "Расчётная скидка",
		"ja":    "推定割引額",
		"vi":    "Ưu đãi ước tính",
	},
	"gross_quota": {
		"en":    "Gross charges",
		"zh":    "累计扣减",
		"zh-TW": "累計扣減",
		"fr":    "Total débité",
		"ru":    "Всего списано",
		"ja":    "累計引き落とし額",
		"vi":    "Tổng đã khấu trừ",
	},
	"refund_quota": {
		"en":    "Refund amount",
		"zh":    "累计退回",
		"zh-TW": "累計退回",
		"fr":    "Total remboursé",
		"ru":    "Всего возвращено",
		"ja":    "累計返金額",
		"vi":    "Tổng đã hoàn lại",
	},
	"net_quota": {
		"en":    "Net amount",
		"zh":    "净额",
		"zh-TW": "淨額",
		"fr":    "Montant net",
		"ru":    "Чистая сумма",
		"ja":    "純額",
		"vi":    "Số tiền thuần",
	},
	"group_ratio": {
		"en":    "Group Ratio",
		"zh":    "分组倍率",
		"zh-TW": "分組倍率",
		"fr":    "Ratio de groupe",
		"ru":    "Групповой коэффициент",
		"ja":    "グループ倍率",
		"vi":    "Tỷ lệ nhóm",
	},
	"contract_applicable": {
		"en":    "Contract applicable",
		"zh":    "适用合同",
		"zh-TW": "適用合同",
		"fr":    "Contrat applicable",
		"ru":    "Применимый контракт",
		"ja":    "適用契約",
		"vi":    "Áp dụng hợp đồng",
	},
	"contract_name": {
		"en":    "Contract name",
		"zh":    "合同名称",
		"zh-TW": "合同名稱",
		"fr":    "Nom du contrat",
		"ru":    "Название контракта",
		"ja":    "契約名",
		"vi":    "Tên hợp đồng",
	},
	"contract_id": {
		"en":    "Contract ID",
		"zh":    "合同 ID",
		"zh-TW": "合同 ID",
		"fr":    "ID de contrat",
		"ru":    "ID контракта",
		"ja":    "契約 ID",
		"vi":    "ID hợp đồng",
	},
	"contract_version": {
		"en":    "Contract version",
		"zh":    "合同版本",
		"zh-TW": "合同版本",
		"fr":    "Version du contrat",
		"ru":    "Версия контракта",
		"ja":    "契約バージョン",
		"vi":    "Phiên bản hợp đồng",
	},
	"contract_ratio": {
		"en":    "Contract discount",
		"zh":    "合同折扣",
		"zh-TW": "合約折扣",
		"fr":    "Remise contractuelle",
		"ru":    "Договорная скидка",
		"ja":    "契約割引",
		"vi":    "Chiết khấu hợp đồng",
	},
	"data_quality": {
		"en":    "Data quality",
		"zh":    "数据质量",
		"zh-TW": "資料品質",
		"fr":    "Qualité des données",
		"ru":    "Качество данных",
		"ja":    "データ品質",
		"vi":    "Chất lượng dữ liệu",
	},
}

// Summary rows have one grain: API Key × customer model × billing mode.
var customerExportSummaryColumnOrder = []string{
	"api_key_name", "api_key_id", "model", "billing_mode", "requests", "currency",
	"net_amount", "original_amount", "discount_amount", "gross_amount", "refund_amount",
	"input_tokens", "cache_read_tokens", "cache_write_tokens", "output_tokens",
	"billable_calls", "refunded_calls", "data_quality", "estimate_reasons",
	"period_start", "period_end", "timezone", "customer_username", "generated_at",
}

func customerExportSummaryHeaderLabel(language string, key string) string {
	if strings.HasSuffix(key, "_quota") {
		return customerExportSummaryMoneyLabel(language, key) + " (quota)"
	}
	if strings.HasSuffix(key, "_amount") {
		return customerExportSummaryMoneyLabel(language, strings.TrimSuffix(key, "_amount")+"_quota")
	}
	return customerExportSummaryMoneyLabel(language, key)
}

func customerExportSummaryMoneyLabel(language string, key string) string {
	if labels, ok := customerExportSummaryLabels[key]; ok {
		if label, ok := labels[language]; ok && label != "" {
			return label
		}
		return labels["en"]
	}
	return key
}
