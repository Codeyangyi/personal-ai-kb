import { createRouter, createWebHistory } from 'vue-router'
import KnowledgeImport from '../pages/KnowledgeImport.vue'
import AISearch from '../pages/AISearch.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      name: 'AISearch',
      component: AISearch
    },
    {
      path: '/import',
      name: 'KnowledgeImport',
      component: KnowledgeImport
    }
  ]
})

export default router

