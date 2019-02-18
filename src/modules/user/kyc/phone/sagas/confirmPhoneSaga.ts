// tslint:disable-next-line
import { call, put } from 'redux-saga/effects';
import { API, RequestOptions } from '../../../../../api';
import { fetchError, fetchSuccess } from '../../../../public/alert';
import {
    SendCodeFetch,
    verifyPhoneData,
    verifyPhoneError,
} from '../actions';

const sessionsConfig: RequestOptions = {
    apiVersion: 'barong',
};

export function* confirmPhoneSaga(action: SendCodeFetch) {
    try {
        const { message } = yield call(API.post(sessionsConfig), '/resource/phones/verify', action.payload);
        yield put(verifyPhoneData({ message }));
        yield put(fetchSuccess('success.phone.confirmed'));
    } catch (error) {
        yield put(verifyPhoneError(error));
        yield put(fetchError(error));
    }
}
