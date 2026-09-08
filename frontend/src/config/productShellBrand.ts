export const PRODUCT_SHELL_BRAND_SCHEMA = 'ProductShellBrandV1'

export const productShellBrand = {
  schemaVersion: PRODUCT_SHELL_BRAND_SCHEMA,
  name: '环枢·零售智能体',
  shortName: '环枢',
  tokens: {
    paper: '#f7f2e9',
    ink: '#132d2d',
    teal: '#147b76',
    risk: '#c65b32',
  },
} as const

type LoginCopy = {
  eyebrow: string
  headline: string
  description: string
  employeeAssistant: string
  operatingAnalysis: string
}

export type RetailAgentHomeCopy = {
  eyebrow: string
  headline: string
  compactHeadline: string
  description: string
  currentWork: string
  employeeDescription: string
  analysisDescription: string
  briefDescription: string
  openBrief: string
  startWork: string
  enterWork: string
  continueWork: string
  employeeRecentWork: string
  analysisRecentWork: string
  noEmployeeRecentWork: string
  noAnalysisRecentWork: string
  handoffHint: string
  contactAdmin: string
  serviceUnavailable: string
  loopTitle: string
  loopDescription: string
  loopSteps: [string, string, string, string, string]
  roadmapTitle: string
  roadmapDescription: string
  current: string
  planned: string
}

const loginCopyByLocale: Record<string, LoginCopy> = {
  'zh-CN': {
    eyebrow: '环枢·零售智能体',
    headline: '让每一次零售经营判断，都有可靠依据。',
    description: '连接企业知识与受治理经营数据，帮助团队从理解问题走向经营判断。',
    employeeAssistant: '员工助理',
    operatingAnalysis: '经营分析',
  },
  'en-US': {
    eyebrow: 'HuanShu Retail Agent',
    headline: 'Ground every retail decision in reliable evidence.',
    description: 'Connect enterprise knowledge with governed operating data, helping teams move from understanding a problem to making an informed decision.',
    employeeAssistant: 'Employee Assistant',
    operatingAnalysis: 'Operating Analysis',
  },
  'ru-RU': {
    eyebrow: 'Розничный агент HuanShu',
    headline: 'Каждое решение в розничной торговле опирается на надёжные данные.',
    description: 'Объединяем корпоративные знания и управляемые операционные данные, чтобы помочь команде перейти от понимания проблемы к обоснованному решению.',
    employeeAssistant: 'Помощник сотрудника',
    operatingAnalysis: 'Операционный анализ',
  },
  'ko-KR': {
    eyebrow: 'HuanShu 리테일 에이전트',
    headline: '모든 리테일 운영 판단에 신뢰할 수 있는 근거를 더합니다.',
    description: '기업 지식과 거버넌스가 적용된 운영 데이터를 연결해 문제 이해에서 근거 있는 판단까지 이어갑니다.',
    employeeAssistant: '직원 지원',
    operatingAnalysis: '운영 분석',
  },
}

export function getProductShellLoginCopy(locale: string): LoginCopy {
  return loginCopyByLocale[locale] || loginCopyByLocale['zh-CN']
}

const homeCopyByLocale: Record<string, RetailAgentHomeCopy> = {
  'zh-CN': {
    briefDescription: '先看本周销售与毛利变化，定位值得优先核查的门店和品类。',
    openBrief: '查看经营简报',
    eyebrow: '环枢·零售智能体',
    headline: '让零售经营中的知识、数据与判断不再分散。',
    compactHeadline: '让知识、数据与判断不再分散。',
    description: '从员工日常问题和经营异常出发，连接企业知识与受治理经营数据，帮助团队看清问题、核验依据，形成更可靠的经营判断。',
    currentWork: '从当前工作开始',
    employeeDescription: '查找企业制度、流程和业务资料，快速获得有来源的答案。',
    analysisDescription: '基于受治理经营数据看指标、找异常，获得可核验的分析。',
    startWork: '打开员工助理',
    enterWork: '进入经营分析',
    continueWork: '继续最近对话',
    employeeRecentWork: '最近对话',
    analysisRecentWork: '最近分析',
    noEmployeeRecentWork: '还没有员工助理对话',
    noAnalysisRecentWork: '还没有经营分析',
    handoffHint: '遇到需要经营数据判断的问题，可带着原问题进入经营分析。',
    contactAdmin: '请联系企业管理员开通经营分析权限',
    serviceUnavailable: '经营分析服务暂不可用',
    loopTitle: '从当前问题，选择工作入口',
    loopDescription: '员工助理解答工作问题，经营简报发现变化，经营分析核查原因。',
    loopSteps: ['发现问题', '分析判断', '人工确认', '执行协同', '结果复盘'],
    roadmapTitle: '持续改进的经营闭环',
    roadmapDescription: '先把问题看清、判断做实，再把确认后的行动持续跟进。',
    current: '当前可用',
    planned: '待业务接入',
  },
  'en-US': {
    briefDescription: 'Review weekly sales and margin changes, then locate stores and categories worth investigating.',
    openBrief: 'Open operating brief',
    eyebrow: 'HuanShu Retail Agent',
    headline: 'Bring retail knowledge, data, and judgment together.',
    compactHeadline: 'Bring knowledge, data, and judgment together.',
    description: 'Start from an everyday question or an operating anomaly. Connect enterprise knowledge with governed operating data to clarify the problem, verify the evidence, and reach a more reliable decision.',
    currentWork: 'Start with today\'s work',
    employeeDescription: 'Find enterprise policies, processes, and business materials, with source-backed answers.',
    analysisDescription: 'Use governed operating data to inspect metrics, find anomalies, and produce verifiable analysis.',
    startWork: 'Open Employee Assistant',
    enterWork: 'Open operating analysis',
    continueWork: 'Continue recent conversation',
    employeeRecentWork: 'Recent conversation',
    analysisRecentWork: 'Recent analysis',
    noEmployeeRecentWork: 'No Employee Assistant conversations yet',
    noAnalysisRecentWork: 'No operating analysis yet',
    handoffHint: 'When a question needs operating data, carry the original question into Operating Analysis.',
    contactAdmin: 'Contact an enterprise administrator for operating analysis access',
    serviceUnavailable: 'Operating analysis is temporarily unavailable',
    loopTitle: 'Choose where to start',
    loopDescription: 'Employee Assistant handles enterprise knowledge; Operating Analysis verifies governed data. Carry the original question across when needed.',
    loopSteps: ['Discover', 'Analyze', 'Confirm', 'Coordinate', 'Review'],
    roadmapTitle: 'A continuously improving operating loop',
    roadmapDescription: 'Clarify the problem, ground the decision, then keep confirmed actions moving and learning from results.',
    current: 'Available now',
    planned: 'Awaiting integration',
  },
  'ru-RU': {
    briefDescription: 'Оцените недельные изменения продаж и маржи и выберите магазины и категории для проверки.',
    openBrief: 'Открыть обзор бизнеса',
    eyebrow: 'Розничный агент HuanShu',
    headline: 'Объедините знания, данные и решения в розничной торговле.',
    compactHeadline: 'Объедините знания, данные и решения.',
    description: 'Начните с повседневного вопроса или операционного отклонения. Объедините корпоративные знания и управляемые данные, чтобы уточнить проблему, проверить основания и принять более надёжное решение.',
    currentWork: 'Начните с текущей работы',
    employeeDescription: 'Находите корпоративные правила, процессы и материалы и получайте ответы со ссылками на источники.',
    analysisDescription: 'Проверяйте показатели и отклонения по управляемым операционным данным и получайте проверяемый анализ.',
    startWork: 'Открыть помощника',
    enterWork: 'Открыть операционный анализ',
    continueWork: 'Продолжить последний диалог',
    employeeRecentWork: 'Последний диалог',
    analysisRecentWork: 'Последний анализ',
    noEmployeeRecentWork: 'Диалогов с помощником пока нет',
    noAnalysisRecentWork: 'Операционного анализа пока нет',
    handoffHint: 'Если вопрос требует операционных данных, перенесите его в операционный анализ.',
    contactAdmin: 'Обратитесь к администратору предприятия для получения доступа',
    serviceUnavailable: 'Операционный анализ временно недоступен',
    loopTitle: 'Выберите, с чего начать',
    loopDescription: 'Помощник работает с корпоративными знаниями, операционный анализ проверяет управляемые данные. При необходимости исходный вопрос можно перенести.',
    loopSteps: ['Выявить', 'Проанализировать', 'Подтвердить', 'Выполнить', 'Оценить результат'],
    roadmapTitle: 'Непрерывный цикл улучшения операций',
    roadmapDescription: 'Уточните проблему, обоснуйте решение, затем сопровождайте подтверждённые действия и учитывайте результат.',
    current: 'Доступно сейчас',
    planned: 'Ожидает интеграции',
  },
  'ko-KR': {
    briefDescription: '주간 매출과 매출총이익 변화를 보고 먼저 확인할 매장과 품목군을 찾습니다.',
    openBrief: '경영 브리핑 보기',
    eyebrow: 'HuanShu 리테일 에이전트',
    headline: '리테일 운영의 지식, 데이터, 판단을 하나로 연결합니다.',
    compactHeadline: '지식, 데이터, 판단을 하나로 연결합니다.',
    description: '일상적인 질문이나 운영 이상에서 시작해 기업 지식과 거버넌스가 적용된 운영 데이터를 연결하고, 문제와 근거를 확인해 더 신뢰할 수 있는 판단을 내립니다.',
    currentWork: '현재 업무에서 시작하기',
    employeeDescription: '기업 규정, 절차, 업무 자료를 찾고 출처가 있는 답변을 빠르게 얻습니다.',
    analysisDescription: '거버넌스가 적용된 운영 데이터로 지표와 이상을 확인하고 검증 가능한 분석을 얻습니다.',
    startWork: '직원 도우미 열기',
    enterWork: '운영 분석 열기',
    continueWork: '최근 대화 계속하기',
    employeeRecentWork: '최근 대화',
    analysisRecentWork: '최근 분석',
    noEmployeeRecentWork: '직원 도우미 대화가 없습니다',
    noAnalysisRecentWork: '운영 분석이 없습니다',
    handoffHint: '운영 데이터 판단이 필요한 질문은 원래 질문과 함께 운영 분석으로 이어갈 수 있습니다.',
    contactAdmin: '운영 분석 권한은 기업 관리자에게 문의하세요',
    serviceUnavailable: '운영 분석 서비스를 일시적으로 사용할 수 없습니다',
    loopTitle: '현재 업무에 맞는 시작점 선택',
    loopDescription: '직원 도우미는 기업 지식을 다루고 운영 분석은 거버넌스 데이터를 검증합니다. 필요하면 원래 질문을 이어갈 수 있습니다.',
    loopSteps: ['문제 감지', '분석 판단', '사람 확인', '실행 협업', '결과 검토'],
    roadmapTitle: '지속적으로 개선되는 운영 루프',
    roadmapDescription: '문제를 분명히 하고 판단의 근거를 확인한 뒤, 승인된 실행을 추적하고 결과에서 학습합니다.',
    current: '현재 사용 가능',
    planned: '업무 연동 대기',
  },
}

export function getRetailAgentHomeCopy(locale: string): RetailAgentHomeCopy {
  return homeCopyByLocale[locale] || homeCopyByLocale['zh-CN']
}

export type EnterpriseAdministrationCopy = {
  eyebrow: string
  title: string
  description: string
  members: string
  storage: string
  serviceLevel: string
  memberQuota: string
  retry: string
  loadFailed: string
  edgeTitle: string
  edgeDescription: string
  edgeNode: string
  availability: string
  connection: string
  dataService: string
  lastSeen: string
  noContact: string
  empty: string
  emptyDescription: string
  unknown: string
  statusUnavailable: string
  refresh: string
  updatedAt: string
  nodeCount: string
  availableCount: string
  telemetryNote: string
  statuses: Record<string, string>
  reasons: Record<string, string>
}

const enterpriseAdministrationCopyByLocale: Record<string, EnterpriseAdministrationCopy> = {
  'zh-CN': {
    eyebrow: '企业管理', title: '企业管理',
    description: '管理企业知识与成员，查看企业数据连接状态。',
    members: '活跃成员', storage: '知识存储', serviceLevel: '服务级别', memberQuota: '成员额度', retry: '重新加载',
    loadFailed: '企业管理信息暂时无法加载',
    edgeTitle: '边缘节点状态',
    edgeDescription: '用于企业数据访问与经营分析；知识库与成员管理不依赖边缘节点。',
    edgeNode: '边缘节点', availability: '可用状态', connection: '节点连接',
    dataService: '数据服务', lastSeen: '最近联系', noContact: '暂无记录',
    empty: '尚未配置边缘节点', emptyDescription: '需要使用企业数据分析时，请联系平台方配置。',
    unknown: '未知', statusUnavailable: '暂未取得边缘节点状态，无法判断是否可用。',
    refresh: '刷新状态', updatedAt: '状态更新于', nodeCount: '节点', availableCount: '可用',
    telemetryNote: '状态来自最近一次心跳；可用表示节点连接与数据服务正常，具体数据访问仍以实际查询为准。',
    statuses: { available: '可用', abnormal: '异常', offline: '离线', disabled: '已停用', unknown: '未知', online: '在线' },
    reasons: {
      available: '', abnormal: '节点已连接，但数据服务异常。',
      offline: '未收到近期节点心跳，该节点的数据暂时无法访问。',
      disabled: '节点已停用，该节点的数据暂时无法访问。',
      unknown: '暂未取得完整节点状态，无法判断数据是否可访问。',
    },
  },
  'en-US': {
    eyebrow: 'Enterprise administration', title: 'Enterprise administration',
    description: 'Manage enterprise knowledge and members, and review data connectivity.',
    members: 'Active members', storage: 'Knowledge storage', serviceLevel: 'Service level', memberQuota: 'Member quota', retry: 'Try again',
    loadFailed: 'Enterprise administration is temporarily unavailable',
    edgeTitle: 'Edge node status',
    edgeDescription: 'For enterprise data access and operating analysis. Knowledge and member management do not depend on edge nodes.',
    edgeNode: 'Edge node', availability: 'Availability', connection: 'Connection',
    dataService: 'Data service', lastSeen: 'Last contact', noContact: 'No record',
    empty: 'No edge nodes configured', emptyDescription: 'Contact the platform to configure enterprise data access.',
    unknown: 'Unknown', statusUnavailable: 'Edge node status could not be obtained. Availability is unknown.',
    refresh: 'Refresh status', updatedAt: 'Updated at', nodeCount: 'Nodes', availableCount: 'Available',
    telemetryNote: 'Status reflects the latest heartbeat. Available means the connection and data service are healthy; actual data access is verified when querying.',
    statuses: { available: 'Available', abnormal: 'Abnormal', offline: 'Offline', disabled: 'Disabled', unknown: 'Unknown', online: 'Online' },
    reasons: {
      available: '', abnormal: 'Connected, but the data service is abnormal.',
      offline: 'No recent heartbeat. Data on this node is temporarily inaccessible.',
      disabled: 'This node is disabled. Its data is temporarily inaccessible.',
      unknown: 'Incomplete node status. Data accessibility is unknown.',
    },
  },
  'ru-RU': {
    eyebrow: 'Управление предприятием', title: 'Управление предприятием',
    description: 'Управление знаниями, участниками и подключением к данным предприятия.',
    members: 'Активные участники', storage: 'Хранилище знаний', serviceLevel: 'Уровень сервиса', memberQuota: 'Лимит участников', retry: 'Повторить',
    loadFailed: 'Информация о предприятии временно недоступна',
    edgeTitle: 'Состояние пограничных узлов',
    edgeDescription: 'Для доступа к данным и операционного анализа. Управление знаниями и участниками не зависит от узлов.',
    edgeNode: 'Пограничный узел', availability: 'Доступность', connection: 'Подключение',
    dataService: 'Сервис данных', lastSeen: 'Последняя связь', noContact: 'Нет записей',
    empty: 'Пограничные узлы не настроены', emptyDescription: 'Для доступа к данным обратитесь к администратору платформы.',
    unknown: 'Неизвестно', statusUnavailable: 'Не удалось получить состояние узлов. Доступность неизвестна.',
    refresh: 'Обновить', updatedAt: 'Обновлено', nodeCount: 'Узлы', availableCount: 'Доступно',
    telemetryNote: 'Состояние основано на последнем сигнале узла. Доступ к конкретным данным проверяется при запросе.',
    statuses: { available: 'Доступен', abnormal: 'Ошибка', offline: 'Не в сети', disabled: 'Отключён', unknown: 'Неизвестно', online: 'В сети' },
    reasons: {
      available: '', abnormal: 'Узел подключён, но сервис данных работает с ошибкой.',
      offline: 'Нет недавнего сигнала. Данные узла временно недоступны.',
      disabled: 'Узел отключён. Его данные временно недоступны.',
      unknown: 'Недостаточно сведений о состоянии узла.',
    },
  },
  'ko-KR': {
    eyebrow: '기업 관리', title: '기업 관리',
    description: '기업 지식과 구성원을 관리하고 데이터 연결 상태를 확인합니다.',
    members: '활성 구성원', storage: '지식 저장공간', serviceLevel: '서비스 수준', memberQuota: '구성원 한도', retry: '다시 불러오기',
    loadFailed: '기업 관리 정보를 불러올 수 없습니다',
    edgeTitle: '에지 노드 상태',
    edgeDescription: '기업 데이터 접근과 운영 분석에 사용됩니다. 지식 및 구성원 관리는 에지 노드에 의존하지 않습니다.',
    edgeNode: '에지 노드', availability: '가용 상태', connection: '노드 연결',
    dataService: '데이터 서비스', lastSeen: '최근 연결', noContact: '기록 없음',
    empty: '설정된 에지 노드가 없습니다', emptyDescription: '기업 데이터 분석이 필요하면 플랫폼에 설정을 요청하세요.',
    unknown: '알 수 없음', statusUnavailable: '에지 노드 상태를 가져오지 못했습니다. 사용 가능 여부를 알 수 없습니다.',
    refresh: '상태 새로고침', updatedAt: '업데이트', nodeCount: '노드', availableCount: '사용 가능',
    telemetryNote: '최근 하트비트 기준 상태입니다. 실제 데이터 접근은 쿼리 시 확인됩니다.',
    statuses: { available: '사용 가능', abnormal: '오류', offline: '오프라인', disabled: '비활성', unknown: '알 수 없음', online: '온라인' },
    reasons: {
      available: '', abnormal: '노드는 연결되었지만 데이터 서비스에 오류가 있습니다.',
      offline: '최근 하트비트가 없습니다. 이 노드의 데이터에 접근할 수 없습니다.',
      disabled: '노드가 비활성화되어 데이터에 접근할 수 없습니다.',
      unknown: '노드 상태 정보가 충분하지 않아 데이터 접근 여부를 알 수 없습니다.',
    },
  },
}

export function getEnterpriseAdministrationCopy(locale: string): EnterpriseAdministrationCopy {
  return enterpriseAdministrationCopyByLocale[locale] || enterpriseAdministrationCopyByLocale['zh-CN']
}
