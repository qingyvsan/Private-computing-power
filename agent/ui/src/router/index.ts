import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      redirect: '/dashboard',
    },
    {
      path: '/setup',
      name: 'Setup',
      component: () => import('../components/setup/SetupWizard.vue'),
    },
    {
      path: '/dashboard',
      name: 'Dashboard',
      component: () => import('../views/DashboardPage.vue'),
    },
    {
      path: '/nodes',
      name: 'Nodes',
      component: () => import('../views/NodesPage.vue'),
    },
    {
      path: '/nodes/:id',
      name: 'NodeDetail',
      component: () => import('../views/NodeDetailPage.vue'),
    },
    {
      path: '/jobs',
      name: 'Jobs',
      component: () => import('../views/JobsPage.vue'),
    },
    {
      path: '/jobs/submit',
      name: 'JobSubmit',
      component: () => import('../views/JobSubmitDialog.vue'),
    },
    {
      path: '/jobs/:id',
      name: 'JobDetail',
      component: () => import('../views/JobDetailPage.vue'),
    },
    {
      path: '/trust',
      name: 'Trust',
      component: () => import('../views/TrustPage.vue'),
    },
    {
      path: '/invite',
      name: 'Invite',
      component: () => import('../views/InvitePage.vue'),
    },
    {
      path: '/settings',
      name: 'Settings',
      component: () => import('../views/SettingsPage.vue'),
    },
  ],
})

export default router