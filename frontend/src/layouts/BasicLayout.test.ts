import {mount} from '@vue/test-utils'
import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import i18n from '../i18n'
import BasicLayout from './BasicLayout.vue'
vi.mock('vue-router', () => ({ useRoute: () => ({ fullPath: '/server' }) }))
beforeEach(()=>{localStorage.clear();vi.stubGlobal('innerWidth',1366)})
afterEach(()=>vi.unstubAllGlobals())
const open=()=>mount(BasicLayout,{global:{plugins:[i18n],stubs:{AppSidebar:true,AppTopbar:true,RouterLink:true,RouterView:true}}})
it('restores bounded sidebar width and collapse controls across remount',async()=>{
 const first=open()
 const handle = first.get('[role="separator"]')
 await handle.trigger('keydown', { key: 'End' })
 await handle.trigger('keydown', { key: 'ArrowLeft' })
 expect((first.get('aside').element as HTMLElement).style.width).toBe('410px')
 await first.get('button').trigger('click')
 expect(first.find('aside').exists()).toBe(false)
 first.unmount()
 const second=open()
 expect(second.get('button').attributes('aria-expanded')).toBe('false')
 expect(second.find('aside').exists()).toBe(false)
 await second.get('button').trigger('click')
 expect(second.get('[role="separator"]').attributes('aria-valuenow')).toBe('410')
 second.unmount()
})
it('starts collapsed on narrow screens with an accessible reopen control',()=>{
 vi.stubGlobal('innerWidth',600)
 const wrapper=open()
 expect(wrapper.find('aside').exists()).toBe(false)
 expect(wrapper.get('button').attributes('aria-label')).toBeTruthy()
 wrapper.unmount()
})
