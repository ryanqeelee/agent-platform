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
  capabilityListLabel: string
  employeeAssistant: string
  operatingAnalysis: string
}

const loginCopyByLocale: Record<string, LoginCopy> = {
  'zh-CN': {
    eyebrow: '面向零售经营的一体化工作空间',
    headline: '让员工助理与经营分析，回到每天的业务现场。',
    description: '在同一个企业空间中，支持员工协作与可信的经营分析。',
    capabilityListLabel: '产品能力',
    employeeAssistant: '员工助理',
    operatingAnalysis: '经营分析',
  },
  'en-US': {
    eyebrow: 'One workspace for retail operations',
    headline: 'Bring employee assistance and operating analysis into daily retail work.',
    description: 'A single enterprise space for employee collaboration and trusted operating analysis.',
    capabilityListLabel: 'Product capabilities',
    employeeAssistant: 'Employee Assistant',
    operatingAnalysis: 'Operating Analysis',
  },
  'ru-RU': {
    eyebrow: 'Единое рабочее пространство для розницы',
    headline: 'Помощник сотрудника и операционный анализ для ежедневной работы в рознице.',
    description: 'Единое пространство предприятия для совместной работы и достоверного операционного анализа.',
    capabilityListLabel: 'Возможности продукта',
    employeeAssistant: 'Помощник сотрудника',
    operatingAnalysis: 'Операционный анализ',
  },
  'ko-KR': {
    eyebrow: '리테일 운영을 위한 하나의 업무 공간',
    headline: '직원 지원과 운영 분석을 매일의 리테일 업무로 가져옵니다.',
    description: '직원 협업과 신뢰할 수 있는 운영 분석을 위한 하나의 엔터프라이즈 공간입니다.',
    capabilityListLabel: '제품 기능',
    employeeAssistant: '직원 지원',
    operatingAnalysis: '운영 분석',
  },
}

export function getProductShellLoginCopy(locale: string): LoginCopy {
  return loginCopyByLocale[locale] || loginCopyByLocale['zh-CN']
}
