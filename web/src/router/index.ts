// 路由表：公开页（登录、微信授权回调）+ MainLayout 包裹的业务页 + 兜底重定向。
import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'

import MainLayout from '@/layouts/MainLayout.vue'
import AuditView from '@/views/AuditView.vue'
import AuthorizeCallbackView from '@/views/AuthorizeCallbackView.vue'
import AuthorizersView from '@/views/AuthorizersView.vue'
import DashboardView from '@/views/DashboardView.vue'
import JobCreateView from '@/views/JobCreateView.vue'
import JobDetailView from '@/views/JobDetailView.vue'
import JobsView from '@/views/JobsView.vue'
import LoginView from '@/views/LoginView.vue'
import LogsView from '@/views/LogsView.vue'
import PreflightView from '@/views/PreflightView.vue'
import ReleaseView from '@/views/ReleaseView.vue'
import SettingsView from '@/views/SettingsView.vue'
import TemplatesView from '@/views/TemplatesView.vue'
import { useAuth } from '@/stores/auth'

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'login',
    component: LoginView,
    meta: { public: true, title: '登录' },
  },
  {
    // 微信第三方平台授权回调落地页：公开访问（微信会带 auth_code 跳到这里）。
    path: '/authorize/callback',
    name: 'authorize-callback',
    component: AuthorizeCallbackView,
    meta: { public: true, title: '授权回调' },
  },
  {
    path: '/',
    component: MainLayout,
    children: [
      { path: '', redirect: '/dashboard' },
      { path: 'dashboard', name: 'dashboard', component: DashboardView, meta: { title: '概览' } },
      {
        path: 'authorizers',
        name: 'authorizers',
        component: AuthorizersView,
        meta: { title: '小程序管理' },
      },
      {
        // 小程序详情：与列表页同组件（后续阶段由列表页内部的详情抽屉/详情面板实现）。
        path: 'authorizers/:appid',
        name: 'authorizer-detail',
        component: AuthorizersView,
        meta: { title: '小程序详情' },
      },
      { path: 'templates', name: 'templates', component: TemplatesView, meta: { title: '代码模板' } },
      { path: 'preflight', name: 'preflight', component: PreflightView, meta: { title: '前置体检' } },
      { path: 'jobs', name: 'jobs', component: JobsView, meta: { title: '批量任务' } },
      { path: 'jobs/new', name: 'job-create', component: JobCreateView, meta: { title: '新建任务' } },
      { path: 'jobs/:id', name: 'job-detail', component: JobDetailView, meta: { title: '任务详情' } },
      { path: 'audits', name: 'audits', component: AuditView, meta: { title: '审核管理' } },
      { path: 'releases', name: 'releases', component: ReleaseView, meta: { title: '发布管理' } },
      { path: 'logs', name: 'logs', component: LogsView, meta: { title: '日志' } },
      { path: 'settings', name: 'settings', component: SettingsView, meta: { title: '设置' } },
    ],
  },
  { path: '/:pathMatch(.*)*', redirect: '/dashboard' },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

// 登录守卫：未登录跳转登录页并保留回跳地址；已登录访问登录页直接回到概览。
router.beforeEach((to) => {
  const { isLoggedIn } = useAuth()
  const requiresAuth = to.meta.public !== true
  if (requiresAuth && !isLoggedIn.value) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.name === 'login' && isLoggedIn.value) {
    return { name: 'dashboard' }
  }
  return true
})

router.afterEach((to) => {
  const title = to.meta.title
  document.title = title ? `${String(title)} · 微信第三方平台管理平台` : '微信第三方平台管理平台'
})

export default router
