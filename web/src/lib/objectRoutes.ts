import type { RouteRecordRaw } from 'vue-router'
import BasicLayout from '../layouts/BasicLayout.vue'
import ObjectsView from '../views/ObjectsView.vue'

export const objectRoutes: RouteRecordRaw[] = [{
 path: '/db/:db/objects/:kind', component: BasicLayout,
 children: [{ path: '', name: 'db-objects', component: ObjectsView }],
}, {
 path: '/db/:db/table/:t/partitions', component: BasicLayout,
 children: [{ path: '', name: 'db-partitions', component: ObjectsView }],
}]
