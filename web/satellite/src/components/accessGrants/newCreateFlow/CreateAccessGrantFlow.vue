// Copyright (C) 2023 Storj Labs, Inc.
// See LICENSE for copying information.

<template>
    <VModal :on-close="closeModal">
        <template #content>
            <div class="modal">
                <div class="modal__header">
                    <component :is="STEP_ICON_AND_TITLE[step].icon" />
                    <h1 class="modal__header__title">{{ STEP_ICON_AND_TITLE[step].title }}</h1>
                </div>
                <CreateNewAccessStep
                    v-if="step === CreateAccessStep.CreateNewAccess"
                    :on-select-type="selectAccessType"
                    :selected-access-types="selectedAccessTypes"
                    :name="accessName"
                    :set-name="setAccessName"
                    :on-continue="() => setStep(CreateAccessStep.ChoosePermission)"
                />
                <ChoosePermissionStep
                    v-if="step === CreateAccessStep.ChoosePermission"
                    :on-select-permission="selectPermissions"
                    :selected-permissions="selectedPermissions"
                    :on-back="() => setStep(CreateAccessStep.CreateNewAccess)"
                    :on-continue="() => setStep(CreateAccessStep.AccessEncryption)"
                    :selected-buckets="selectedBuckets"
                    :on-select-bucket="selectBucket"
                    :on-select-all-buckets="selectAllBuckets"
                    :on-unselect-bucket="unselectBucket"
                    :not-after="notAfter"
                    :on-set-not-after="setNotAfter"
                    :not-after-label="notAfterLabel"
                    :on-set-not-after-label="setNotAfterLabel"
                />
            </div>
        </template>
    </VModal>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';

import { useNotify, useRoute, useRouter, useStore } from '@/utils/hooks';
import { RouteConfig } from '@/router';
import { AccessType, CreateAccessStep, Permission, STEP_ICON_AND_TITLE } from '@/types/createAccessGrant';
import { BUCKET_ACTIONS } from '@/store/modules/buckets';
import { AnalyticsErrorEventSource } from '@/utils/constants/analyticsEventNames';

import VModal from '@/components/common/VModal.vue';
import CreateNewAccessStep from '@/components/accessGrants/newCreateFlow/steps/CreateNewAccessStep.vue';
import ChoosePermissionStep from '@/components/accessGrants/newCreateFlow/steps/ChoosePermissionStep.vue';

const router = useRouter();
const route = useRoute();
const notify = useNotify();
const store = useStore();

const initPermissions = [
    Permission.Read,
    Permission.Write,
    Permission.Delete,
    Permission.List,
];

const step = ref<CreateAccessStep>(CreateAccessStep.CreateNewAccess);
const selectedAccessTypes = ref<AccessType[]>([]);
const selectedPermissions = ref<Permission[]>(initPermissions);
const selectedBuckets = ref<string[]>([]);
const accessName = ref<string>('');
const notAfter = ref<Date | undefined>(undefined);
const notAfterLabel = ref<string>('No end date');

/**
 * Selects access type.
 */
function selectAccessType(type: AccessType) {
    // "access grant" and "s3 credentials" can be selected at the same time,
    // but "API key" cannot be selected if either "access grant" or "s3 credentials" is selected.
    switch (type) {
    case AccessType.AccessGrant:
        // Unselect API key if was selected.
        unselectAPIKeyAccessType();

        // Unselect Access grant if was selected.
        if (selectedAccessTypes.value.includes(AccessType.AccessGrant)) {
            selectedAccessTypes.value = selectedAccessTypes.value.filter(t => t !== AccessType.AccessGrant);
            return;
        }

        // Select Access grant.
        selectedAccessTypes.value.push(type);
        break;
    case AccessType.S3:
        // Unselect API key if was selected.
        unselectAPIKeyAccessType();

        // Unselect S3 if was selected.
        if (selectedAccessTypes.value.includes(AccessType.S3)) {
            selectedAccessTypes.value = selectedAccessTypes.value.filter(t => t !== AccessType.S3);
            return;
        }

        // Select S3.
        selectedAccessTypes.value.push(type);
        break;
    case AccessType.APIKey:
        // Unselect Access grant and S3 if were selected.
        if (selectedAccessTypes.value.includes(AccessType.AccessGrant) || selectedAccessTypes.value.includes(AccessType.S3)) {
            selectedAccessTypes.value = selectedAccessTypes.value.filter(t => t === AccessType.APIKey);
        }

        // Unselect API key if was selected.
        if (selectedAccessTypes.value.includes(AccessType.APIKey)) {
            selectedAccessTypes.value = selectedAccessTypes.value.filter(t => t !== AccessType.APIKey);
            return;
        }

        // Select API key.
        selectedAccessTypes.value.push(type);
    }
}

/**
 * Sets not after (end date) caveat.
 */
function setNotAfter(date: Date | undefined): void {
    notAfter.value = date;
}

/**
 * Sets not after (end date) label.
 */
function setNotAfterLabel(label: string): void {
    notAfterLabel.value = label;
}

/**
 * Unselects API key access type.
 */
function unselectAPIKeyAccessType(): void {
    if (selectedAccessTypes.value.includes(AccessType.APIKey)) {
        selectedAccessTypes.value = selectedAccessTypes.value.filter(t => t !== AccessType.APIKey);
    }
}

/**
 * Selects access grant permissions.
 */
function selectPermissions(permission: Permission): void {
    switch (permission) {
    case Permission.All:
        if (selectedPermissions.value.length === 4) {
            selectedPermissions.value = [];
            return;
        }

        selectedPermissions.value = initPermissions;
        break;
    case Permission.Delete:
        handlePermissionSelection(Permission.Delete);
        break;
    case Permission.List:
        handlePermissionSelection(Permission.List);
        break;
    case Permission.Write:
        handlePermissionSelection(Permission.Write);
        break;
    case Permission.Read:
        handlePermissionSelection(Permission.Read);
    }
}

/**
 * Handles permission select/unselect.
 */
function handlePermissionSelection(permission: Permission) {
    if (selectedPermissions.value.includes(permission)) {
        selectedPermissions.value = selectedPermissions.value.filter(p => p !== permission);
        return;
    }

    selectedPermissions.value.push(permission);
}

/**
 * Clears bucket selection which means grant access to all buckets.
 */
function selectAllBuckets() {
    selectedBuckets.value = [];
}

/**
 * Select some specific bucket.
 */
function selectBucket(bucket: string) {
    selectedBuckets.value.push(bucket);
}

/**
 * Unselect some specific bucket.
 */
function unselectBucket(bucket: string) {
    selectedBuckets.value = selectedBuckets.value.filter(b => b !== bucket);
}

/**
 * Sets access grant name from input field.
 * @param value
 */
function setAccessName(value: string): void {
    accessName.value = value;
}

/**
 * Sets current step to be 'Choose permission'.
 */
function setStep(stepArg: CreateAccessStep): void {
    step.value = stepArg;
}

/**
 * Closes create access grant flow.
 */
function closeModal(): void {
    router.push(RouteConfig.AccessGrants.path);
}

onMounted(async () => {
    if (route.params?.accessType) {
        selectedAccessTypes.value.push(route.params?.accessType as AccessType);
    }

    try {
        await store.dispatch(BUCKET_ACTIONS.FETCH_ALL_BUCKET_NAMES);
    } catch (error) {
        notify.error(`Unable to fetch all bucket names. ${error.message}`, AnalyticsErrorEventSource.CREATE_AG_MODAL);
    }
});
</script>

<style scoped lang="scss">
.modal {
    width: 346px;
    padding: 32px;
    display: flex;
    flex-direction: column;

    &__header {
        display: flex;
        align-items: center;
        padding-bottom: 16px;
        border-bottom: 1px solid #ebeef1;

        &__title {
            margin-left: 16px;
            font-family: 'font_bold', sans-serif;
            font-size: 24px;
            line-height: 31px;
            letter-spacing: -0.02em;
            color: #000;
        }
    }
}
</style>
