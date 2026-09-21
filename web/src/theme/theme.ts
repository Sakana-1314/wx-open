// NaiveUI 蓝色主题：除警示语义色外统一蓝色系。
import type { GlobalThemeOverrides } from 'naive-ui'

/** 主蓝。 */
export const PRIMARY_COLOR = '#1668dc'
export const PRIMARY_HOVER = '#4098fc'
export const PRIMARY_PRESSED = '#0f5ab9'
export const PRIMARY_SUPPL = '#d9e8fb'

export const themeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: PRIMARY_COLOR,
    primaryColorHover: PRIMARY_HOVER,
    primaryColorPressed: PRIMARY_PRESSED,
    primaryColorSuppl: PRIMARY_SUPPL,
    borderRadius: '6px',
  },
  Button: {
    fontWeight: '500',
  },
  Menu: {
    itemTextColorActive: PRIMARY_COLOR,
    itemIconColorActive: PRIMARY_COLOR,
    itemColorActive: PRIMARY_SUPPL,
    itemColorActiveHover: PRIMARY_SUPPL,
    itemColorHover: 'rgba(22, 104, 220, 0.08)',
  },
  Layout: {
    siderColor: '#ffffff',
    headerColor: '#ffffff',
    color: '#f0f4fa',
  },
  Card: {
    borderColor: '#dbe5f1',
  },
  DataTable: {
    thColor: '#f2f7fd',
    thTextColor: '#1f2937',
    borderColor: '#e2e9f2',
  },
  Tabs: {
    tabTextColorActive: PRIMARY_COLOR,
    tabColorSegment: PRIMARY_SUPPL,
    // 去掉 line 型页签导航底部通栏分割线（其下紧邻列表工具条）
    tabBorderColor: 'transparent',
  },
  Tag: {
    borderRadius: '4px',
  },
}