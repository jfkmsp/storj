// Copyright (C) 2023 Storj Labs, Inc.
// See LICENSE for copying information.

import { VueConstructor } from 'vue';

import CreateNewAccessIcon from '@/../static/images/accessGrants/newCreateFlow/createNewAccess.svg';
import ChoosePermissionIcon from '@/../static/images/accessGrants/newCreateFlow/choosePermission.svg';
import AccessEncryptionIcon from '@/../static/images/accessGrants/newCreateFlow/accessEncryption.svg';
import PassphraseGeneratedIcon from '@/../static/images/accessGrants/newCreateFlow/passphraseGenerated.svg';
import AccessCreatedIcon from '@/../static/images/accessGrants/newCreateFlow/accessCreated.svg';
import CredentialsCreatedIcon from '@/../static/images/accessGrants/newCreateFlow/credentialsCreated.svg';
import EncryptionInfoIcon from '@/../static/images/accessGrants/newCreateFlow/encryptionInfo.svg';
import TypeIcon from '@/../static/images/accessGrants/newCreateFlow/typeIcon.svg';
import NameIcon from '@/../static/images/accessGrants/newCreateFlow/nameIcon.svg';
import PermissionsIcon from '@/../static/images/accessGrants/newCreateFlow/permissionsIcon.svg';
import BucketsIcon from '@/../static/images/accessGrants/newCreateFlow/bucketsIcon.svg';
import EndDateIcon from '@/../static/images/accessGrants/newCreateFlow/endDateIcon.svg';
import EncryptionPassphraseIcon from '@/../static/images/accessGrants/newCreateFlow/encryptionPassphraseIcon.svg';

export interface IconAndTitle {
    icon: VueConstructor;
    title: string;
}

export enum AccessType {
    APIKey = 'apikey',
    S3 = 's3',
    AccessGrant = 'accessGrant',
}

export enum CreateAccessStep {
    CreateNewAccess = 'createNewAccess',
    ChoosePermission = 'choosePermission',
    EncryptionInfo = 'encryptionInfo',
    AccessEncryption = 'accessEncryption',
    PassphraseGenerated = 'passphraseGenerated',
    EnterNewPassphrase = 'enterNewPassphrase',
    AccessCreated = 'accessCreated',
    CredentialsCreated = 'credentialsCreated',
}

export enum Permission {
    All = 'all',
    Read = 'read',
    Write = 'write',
    List = 'list',
    Delete = 'delete',
}

export const STEP_ICON_AND_TITLE: Record<CreateAccessStep, IconAndTitle> = {
    [CreateAccessStep.CreateNewAccess]: {
        icon: CreateNewAccessIcon,
        title: 'Create a new access',
    },
    [CreateAccessStep.ChoosePermission]: {
        icon: ChoosePermissionIcon,
        title: 'Choose permissions',
    },
    [CreateAccessStep.EncryptionInfo]: {
        icon: EncryptionInfoIcon,
        title: 'Encryption information',
    },
    [CreateAccessStep.AccessEncryption]: {
        icon: AccessEncryptionIcon,
        title: 'Access encryption',
    },
    [CreateAccessStep.PassphraseGenerated]: {
        icon: PassphraseGeneratedIcon,
        title: 'Passphrase generated',
    },
    [CreateAccessStep.EnterNewPassphrase]: {
        icon: AccessEncryptionIcon,
        title: 'Enter a new passphrase',
    },
    [CreateAccessStep.AccessCreated]: {
        icon: AccessCreatedIcon,
        title: 'Access created',
    },
    [CreateAccessStep.CredentialsCreated]: {
        icon: CredentialsCreatedIcon,
        title: 'Credentials created',
    },
};

export enum FunctionalContainer {
    Type = 'type',
    Name = 'name',
    Permissions = 'permissions',
    Buckets = 'buckets',
    EndDate = 'endDate',
    EncryptionPassphrase = 'encryptionPassphrase',
}

export const FUNCTIONAL_CONTAINER_ICON_AND_TITLE: Record<FunctionalContainer, IconAndTitle> = {
    [FunctionalContainer.Type]: {
        icon: TypeIcon,
        title: 'Type',
    },
    [FunctionalContainer.Name]: {
        icon: NameIcon,
        title: 'Name',
    },
    [FunctionalContainer.Permissions]: {
        icon: PermissionsIcon,
        title: 'Permissions',
    },
    [FunctionalContainer.Buckets]: {
        icon: BucketsIcon,
        title: 'Buckets',
    },
    [FunctionalContainer.EndDate]: {
        icon: EndDateIcon,
        title: 'End Date',
    },
    [FunctionalContainer.EncryptionPassphrase]: {
        icon: EncryptionPassphraseIcon,
        title: 'Encryption Passphrase',
    },
};
